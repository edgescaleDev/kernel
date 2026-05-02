package kernel

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/edgescaleDev/kernel/internal"
	"github.com/edgescaleDev/kernel/sdk"
)

// cronInfo is the JSON shape returned by the admin cron endpoints.
type cronInfo struct {
	ID          string                `json:"id"`
	Module      string                `json:"module"`
	Schedule    string                `json:"schedule"`
	Timezone    string                `json:"timezone"`
	Timeout     string                `json:"timeout"`
	Description sdk.TranslatableField `json:"description"`
	Paused      bool                  `json:"paused"`
	RetryPolicy *retryPolicyInfo      `json:"retry_policy,omitempty"`
}

type retryPolicyInfo struct {
	MaxAttempts int    `json:"max_attempts"`
	Backoff     string `json:"backoff"`
}

// executionInfo is the JSON shape for execution history records.
type executionInfo struct {
	ID           string     `json:"id"`
	CronID       string     `json:"cron_id"`
	InstanceID   string     `json:"instance_id"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Status       string     `json:"status"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	Attempt      int        `json:"attempt"`
}

// handleListCrons returns all registered crons with their current paused status.
// GET /_kernel/crons
func (k *Kernel) handleListCrons(c *gin.Context) {
	var crons []cronInfo

	for _, m := range k.orderedModules() {
		manifest := m.Manifest()
		for _, def := range manifest.Crons {
			qualifiedID := manifest.ID + "." + def.ID
			tz := def.Timezone
			if tz == "" {
				tz = "UTC"
			}
			timeout := def.Timeout
			if timeout == 0 {
				timeout = 5 * time.Minute
			}

			info := cronInfo{
				ID:          qualifiedID,
				Module:      manifest.ID,
				Schedule:    def.Schedule,
				Timezone:    tz,
				Timeout:     timeout.String(),
				Description: def.Description,
				Paused:      k.isCronPaused(c.Request.Context(), qualifiedID),
			}

			if def.Retry != nil {
				info.RetryPolicy = &retryPolicyInfo{
					MaxAttempts: def.Retry.MaxAttempts,
					Backoff:     def.Retry.Backoff.String(),
				}
			}

			crons = append(crons, info)
		}
	}

	sdk.OK(c, crons)
}

// handleCronExecutions returns paginated execution history for a cron.
// GET /_kernel/crons/:id/executions
func (k *Kernel) handleCronExecutions(c *gin.Context) {
	cronID := c.Param("id")
	if cronID == "" {
		sdk.FromError(c, sdk.BadRequest("cron ID is required"))
		return
	}

	page := sdk.ParsePageRequest(c)
	result, err := sdk.Paginate[internal.CronExecution](
		k.db.Where("cron_id = ?", cronID).Order("started_at DESC"),
		page,
	)
	if err != nil {
		sdk.FromError(c, err)
		return
	}

	// Map to response shape.
	items := make([]executionInfo, len(result.Items))
	for i, exec := range result.Items {
		items[i] = executionInfo{
			ID:           exec.ID,
			CronID:       exec.CronID,
			InstanceID:   exec.InstanceID,
			StartedAt:    exec.StartedAt,
			FinishedAt:   exec.FinishedAt,
			Status:       exec.Status,
			ErrorMessage: exec.ErrorMessage,
			Attempt:      exec.Attempt,
		}
	}

	sdk.List(c, items, result.Meta)
}

// handlePauseCron pauses a cron job by setting a Redis flag.
// POST /_kernel/crons/:id/pause
func (k *Kernel) handlePauseCron(c *gin.Context) {
	cronID := c.Param("id")
	if cronID == "" {
		sdk.FromError(c, sdk.BadRequest("cron ID is required"))
		return
	}

	if k.redis == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "redis not configured"})
		return
	}

	if err := k.redis.Set(c.Request.Context(), "cron:"+cronID+":paused", "true", 0).Err(); err != nil {
		sdk.FromError(c, fmt.Errorf("pause cron: %w", err))
		return
	}

	k.logger.Info("cron paused", "cron", cronID)
	sdk.OK(c, gin.H{"id": cronID, "paused": true})
}

// handleResumeCron resumes a paused cron job by removing the Redis flag.
// POST /_kernel/crons/:id/resume
func (k *Kernel) handleResumeCron(c *gin.Context) {
	cronID := c.Param("id")
	if cronID == "" {
		sdk.FromError(c, sdk.BadRequest("cron ID is required"))
		return
	}

	if k.redis == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "redis not configured"})
		return
	}

	if err := k.redis.Del(c.Request.Context(), "cron:"+cronID+":paused").Err(); err != nil {
		sdk.FromError(c, fmt.Errorf("resume cron: %w", err))
		return
	}

	k.logger.Info("cron resumed", "cron", cronID)
	sdk.OK(c, gin.H{"id": cronID, "paused": false})
}

// handleTriggerCron triggers an immediate execution of a cron via Redis pub/sub.
// POST /_kernel/crons/:id/trigger
func (k *Kernel) handleTriggerCron(c *gin.Context) {
	cronID := c.Param("id")
	if cronID == "" {
		sdk.FromError(c, sdk.BadRequest("cron ID is required"))
		return
	}

	if k.redis == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "redis not configured"})
		return
	}

	if err := k.redis.Publish(c.Request.Context(), "cron:trigger:"+cronID, "trigger").Err(); err != nil {
		sdk.FromError(c, fmt.Errorf("trigger cron: %w", err))
		return
	}

	k.logger.Info("cron triggered", "cron", cronID)
	sdk.OK(c, gin.H{"id": cronID, "triggered": true})
}

// isCronPaused checks the Redis pause flag for a cron.
func (k *Kernel) isCronPaused(ctx context.Context, cronID string) bool {
	if k.redis == nil {
		return false
	}
	val, err := k.redis.Get(ctx, "cron:"+cronID+":paused").Result()
	if err != nil {
		return false
	}
	return val == "true"
}
