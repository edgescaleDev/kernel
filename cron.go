package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/edgescaleDev/kernel/internal"
	"github.com/kernel-contrib/sdk"
)

// cronEntry is a fully-resolved cron job ready for scheduling.
// Built during initCronMode from Manifest.Crons + RegisterCrons handlers.
type cronEntry struct {
	def         sdk.CronDef
	moduleID    string
	qualifiedID string // "shifts.reminders"
	handler     sdk.CronHandler
}

// cronRunner manages the gocron scheduler and cron lifecycle.
type cronRunner struct {
	scheduler  gocron.Scheduler
	entries    []cronEntry
	db         *gorm.DB
	redis      *redis.Client
	logger     *slog.Logger
	instanceID string

	// baseCtx is derived from the ctx passed to start() and is canceled in stop().
	// All background goroutines and per-job contexts are derived from it, so that
	// kernel shutdown can interrupt long-running jobs.
	baseCtx    context.Context
	cancelBase context.CancelFunc
}

// newCronRunner creates a new cron runner with gocron integration.
func newCronRunner(db *gorm.DB, rdb *redis.Client, lock sdk.LockProvider, logger *slog.Logger) (*cronRunner, error) {
	instanceID := fmt.Sprintf("%s-%d", hostname(), os.Getpid())

	opts := []gocron.SchedulerOption{
		gocron.WithLogger(&slogAdapter{logger: logger}),
		gocron.WithGlobalJobOptions(
			gocron.WithSingletonMode(gocron.LimitModeReschedule),
		),
	}

	// Wire distributed locker if a LockProvider is configured.
	if lock != nil {
		opts = append(opts, gocron.WithDistributedLocker(&lockerAdapter{provider: lock}))
	}

	s, err := gocron.NewScheduler(opts...)
	if err != nil {
		return nil, fmt.Errorf("create scheduler: %w", err)
	}

	return &cronRunner{
		scheduler:  s,
		db:         db,
		redis:      rdb,
		logger:     logger,
		instanceID: instanceID,
	}, nil
}

// register adds a cron entry to the runner. Called before start().
func (cr *cronRunner) register(entry cronEntry) {
	cr.entries = append(cr.entries, entry)
}

// start registers all jobs with gocron and starts the scheduler.
func (cr *cronRunner) start(ctx context.Context) error {
	// Create a base context for the lifetime of the runner. Canceling it
	// stops the heartbeat, trigger listener, and interrupts in-flight jobs.
	cr.baseCtx, cr.cancelBase = context.WithCancel(ctx)

	for _, entry := range cr.entries {
		if err := cr.addJob(entry); err != nil {
			cr.cancelBase()
			return fmt.Errorf("register cron %q: %w", entry.qualifiedID, err)
		}
	}

	cr.scheduler.Start()
	cr.logger.Info("cron runner started",
		"jobs", len(cr.entries),
		"instance", cr.instanceID,
	)

	// Start background goroutines bound to the runner's base context.
	go cr.heartbeat(cr.baseCtx)
	go cr.listenForTriggers(cr.baseCtx)

	return nil
}

// addJob registers a single cron entry with the gocron scheduler.
func (cr *cronRunner) addJob(entry cronEntry) error {
	// Resolve timezone.
	loc := time.UTC
	if entry.def.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(entry.def.Timezone)
		if err != nil {
			return fmt.Errorf("invalid timezone %q: %w", entry.def.Timezone, err)
		}
	}

	// Determine job definition type.
	var jobDef gocron.JobDefinition
	if durStr, ok := strings.CutPrefix(entry.def.Schedule, "@every "); ok {
		// @every shorthand — parse as duration job.
		dur, err := time.ParseDuration(durStr)
		if err != nil {
			return fmt.Errorf("invalid @every duration %q: %w", entry.def.Schedule, err)
		}
		jobDef = gocron.DurationJob(dur)
	} else {
		// Standard cron expression (5 or 6 field) or other @ shortcuts (@daily, @weekly, etc.).
		// Embed the timezone via CRON_TZ so gocron evaluates the schedule in the right location.
		schedule := entry.def.Schedule
		if loc != time.UTC {
			schedule = fmt.Sprintf("CRON_TZ=%s %s", loc.String(), schedule)
		}
		// withSeconds=true enables optional 6-field parsing.
		jobDef = gocron.CronJob(schedule, true)
	}

	// Build job options.
	jobOpts := []gocron.JobOption{
		gocron.WithName(entry.qualifiedID),
		gocron.WithTags(entry.moduleID),
		gocron.WithEventListeners(
			gocron.AfterJobRuns(cr.onSuccess(entry)),
			gocron.AfterJobRunsWithError(cr.onError(entry)),
		),
	}

	// Create the task with our wrapped handler.
	_, err := cr.scheduler.NewJob(
		jobDef,
		gocron.NewTask(cr.wrapHandler(entry)),
		jobOpts...,
	)
	return err
}

// wrapHandler wraps a sdk.CronHandler with pause checking, timeout,
// and execution recording.
func (cr *cronRunner) wrapHandler(entry cronEntry) func() {
	return func() {
		// Check if this cron is paused.
		if cr.isPaused(cr.baseCtx, entry.qualifiedID) {
			cr.logger.Info("cron skipped (paused)", "cron", entry.qualifiedID)
			return
		}

		// Set timeout, layered on the runner's base context so that kernel
		// shutdown can interrupt long-running jobs.
		timeout := entry.def.Timeout
		if timeout == 0 {
			timeout = 5 * time.Minute
		}
		ctx, cancel := context.WithTimeout(cr.baseCtx, timeout)
		defer cancel()

		startedAt := time.Now()

		// Record the execution start.
		execID := cr.recordStart(entry, startedAt)

		// Execute the handler with retry support.
		maxAttempts := 1
		backoff := time.Duration(0)
		if entry.def.Retry != nil && entry.def.Retry.MaxAttempts > 0 {
			maxAttempts = entry.def.Retry.MaxAttempts + 1 // +1 for the initial attempt
			backoff = entry.def.Retry.Backoff
		}

		var lastErr error
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			if attempt > 1 {
				cr.logger.Info("retrying cron",
					"cron", entry.qualifiedID,
					"attempt", attempt,
					"backoff", backoff,
				)
				time.Sleep(backoff)
			}

			lastErr = entry.handler(ctx)
			if lastErr == nil {
				cr.recordFinish(execID, "success", nil, attempt)
				return
			}

			cr.logger.Error("cron execution failed",
				"cron", entry.qualifiedID,
				"attempt", attempt,
				"error", lastErr,
			)
		}

		// All attempts exhausted.
		status := "failed"
		if ctx.Err() == context.DeadlineExceeded {
			status = "timeout"
		}
		cr.recordFinish(execID, status, lastErr, maxAttempts)
	}
}

// onSuccess is called by gocron after a job completes without panic.
func (cr *cronRunner) onSuccess(entry cronEntry) func(jobID uuid.UUID, jobName string) {
	return func(jobID uuid.UUID, jobName string) {
		cr.logger.Info("cron completed", "cron", entry.qualifiedID, "job_id", jobID)
	}
}

// onError is called by gocron if the job wrapper panics or gocron encounters an error.
func (cr *cronRunner) onError(entry cronEntry) func(jobID uuid.UUID, jobName string, err error) {
	return func(jobID uuid.UUID, jobName string, err error) {
		cr.logger.Error("cron runner error", "cron", entry.qualifiedID, "job_id", jobID, "error", err)
	}
}

// recordStart inserts a running execution record and returns its ID.
func (cr *cronRunner) recordStart(entry cronEntry, startedAt time.Time) string {
	exec := internal.CronExecution{
		ID:         uuid.New().String(),
		CronID:     entry.qualifiedID,
		InstanceID: cr.instanceID,
		StartedAt:  startedAt,
		Status:     "running",
		Attempt:    1,
	}
	if err := cr.db.Create(&exec).Error; err != nil {
		cr.logger.Error("failed to record cron start", "cron", entry.qualifiedID, "error", err)
	}
	return exec.ID
}

// recordFinish updates an execution record with the final status.
func (cr *cronRunner) recordFinish(execID string, status string, err error, attempt int) {
	now := time.Now()
	updates := map[string]any{
		"status":      status,
		"finished_at": now,
		"attempt":     attempt,
	}
	if err != nil {
		errMsg := err.Error()
		updates["error_message"] = errMsg
	}

	if dbErr := cr.db.Model(&internal.CronExecution{}).Where("id = ?", execID).Updates(updates).Error; dbErr != nil {
		cr.logger.Error("failed to record cron finish", "exec_id", execID, "error", dbErr)
	}
}

// isPaused checks whether a cron is paused via Redis flag.
func (cr *cronRunner) isPaused(ctx context.Context, cronID string) bool {
	if cr.redis == nil {
		return false
	}
	val, err := cr.redis.Get(ctx, "cron:"+cronID+":paused").Result()
	if err != nil {
		return false
	}
	return val == "true"
}

// triggerJob runs the named job immediately via gocron's RunNow.
func (cr *cronRunner) triggerJob(qualifiedID string) error {
	for _, j := range cr.scheduler.Jobs() {
		if j.Name() == qualifiedID {
			return j.RunNow()
		}
	}
	return fmt.Errorf("cron job %q not found in scheduler", qualifiedID)
}

// listenForTriggers subscribes to the Redis pattern "cron:trigger:*" and
// invokes the matching job immediately when a trigger message arrives.
// This is the counterpart to handleTriggerCron which publishes to the channel.
func (cr *cronRunner) listenForTriggers(ctx context.Context) {
	if cr.redis == nil {
		return
	}

	pubsub := cr.redis.PSubscribe(ctx, "cron:trigger:*")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// Extract the cronID from the channel name "cron:trigger:<cronID>".
			cronID := strings.TrimPrefix(msg.Channel, "cron:trigger:")
			if err := cr.triggerJob(cronID); err != nil {
				cr.logger.Error("cron trigger failed", "cron", cronID, "error", err)
			} else {
				cr.logger.Info("cron triggered via pub/sub", "cron", cronID)
			}
		}
	}
}

// heartbeat writes a periodic heartbeat to Redis for CLI health checks.
func (cr *cronRunner) heartbeat(ctx context.Context) {
	if cr.redis == nil {
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	hbKey := "cron:heartbeat:" + cr.instanceID

	// Write initial heartbeat.
	cr.redis.Set(ctx, hbKey, time.Now().Unix(), 30*time.Second)

	for {
		select {
		case <-ctx.Done():
			// Clean up heartbeat on shutdown.
			cr.redis.Del(context.Background(), hbKey)
			return
		case <-ticker.C:
			cr.redis.Set(ctx, hbKey, time.Now().Unix(), 30*time.Second)
		}
	}
}

// stop performs graceful shutdown of the cron scheduler.
func (cr *cronRunner) stop(ctx context.Context) error {
	cr.logger.Info("stopping cron runner")

	// Cancel base context to stop heartbeat and trigger listener goroutines,
	// and to interrupt any in-flight job contexts.
	if cr.cancelBase != nil {
		cr.cancelBase()
	}

	// Shutdown gocron and bound the wait by ctx.
	errCh := make(chan error, 1)
	go func() {
		errCh <- cr.scheduler.Shutdown()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("cron scheduler shutdown: %w", err)
		}
	case <-ctx.Done():
		return fmt.Errorf("cron scheduler shutdown canceled: %w", ctx.Err())
	}

	cr.logger.Info("cron runner stopped")
	return nil
}

// ── gocron adapter: sdk.LockProvider → gocron.Locker ────────────────────────

// lockerAdapter bridges sdk.LockProvider to gocron.Locker.
type lockerAdapter struct {
	provider sdk.LockProvider
}

func (a *lockerAdapter) Lock(ctx context.Context, key string) (gocron.Lock, error) {
	release, acquired, err := a.provider.Acquire(ctx, "cron:"+key, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("lock acquire: %w", err)
	}
	if !acquired {
		return nil, fmt.Errorf("lock not acquired for %q", key)
	}
	return &lockHandle{release: release}, nil
}

// lockHandle implements gocron.Lock.
type lockHandle struct {
	release func()
}

func (l *lockHandle) Unlock(_ context.Context) error {
	l.release()
	return nil
}

// ── gocron adapter: slog → gocron.Logger ────────────────────────────────────

// slogAdapter bridges slog.Logger to gocron.Logger interface.
type slogAdapter struct {
	logger *slog.Logger
}

func (a *slogAdapter) Debug(msg string, args ...any) {
	a.logger.Debug(msg, args...)
}

func (a *slogAdapter) Error(msg string, args ...any) {
	a.logger.Error(msg, args...)
}

func (a *slogAdapter) Info(msg string, args ...any) {
	a.logger.Info(msg, args...)
}

func (a *slogAdapter) Warn(msg string, args ...any) {
	a.logger.Warn(msg, args...)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
