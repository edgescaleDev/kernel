package kernel

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"go.edgescale.dev/kernel/sdk"
	"gorm.io/gorm"
)

// outboxWriter is the kernel's implementation of sdk.OutboxWriter.
// It writes events into the outbox table within the caller's DB context.
type outboxWriter struct {
	db       *gorm.DB
	moduleID string
}

func (w *outboxWriter) WriteEvent(ctx context.Context, subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	event := sdk.OutboxEvent{
		Subject:   subject,
		Payload:   data,
		ServiceID: w.moduleID,
	}
	return w.db.WithContext(ctx).Create(&event).Error
}

// OutboxPoller polls the outbox table for undelivered events and dispatches
// them to the EventBus. Runs as a background goroutine.
type OutboxPoller struct {
	db       *gorm.DB
	bus      sdk.EventBus
	logger   *slog.Logger
	interval time.Duration

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewOutboxPoller creates a new poller that checks for undelivered outbox events.
func NewOutboxPoller(db *gorm.DB, bus sdk.EventBus, logger *slog.Logger) *OutboxPoller {
	return &OutboxPoller{
		db:       db,
		bus:      bus,
		logger:   logger,
		interval: 5 * time.Second,
	}
}

// Start begins the background polling loop.
func (p *OutboxPoller) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.poll(ctx)
			}
		}
	}()

	p.logger.Info("outbox poller started", "interval", p.interval)
}

// Stop gracefully shuts down the poller.
func (p *OutboxPoller) Stop() {
	if p.cancel != nil {
		p.cancel()
		p.wg.Wait()
		p.logger.Info("outbox poller stopped")
	}
}

// poll fetches undelivered events and dispatches them.
func (p *OutboxPoller) poll(ctx context.Context) {
	if p.db == nil {
		return
	}
	var events []sdk.OutboxEvent
	result := p.db.WithContext(ctx).
		Where("status = ?", "pending").
		Order("id ASC").
		Limit(100).
		Find(&events)

	if result.Error != nil {
		p.logger.Error("outbox poll failed", "error", result.Error)
		return
	}

	for _, event := range events {
		if err := p.bus.Publish(ctx, event.Subject, json.RawMessage(event.Payload)); err != nil {
			p.logger.Error("outbox dispatch failed",
				"subject", event.Subject,
				"id", event.ID,
				"error", err,
			)
			// Increment attempt count.
			p.db.Model(&sdk.OutboxEvent{}).Where("id = ?", event.ID).
				UpdateColumn("attempts", gorm.Expr("attempts + 1"))
			continue
		}

		// Mark as delivered.
		p.db.Model(&sdk.OutboxEvent{}).Where("id = ?", event.ID).
			Update("status", "delivered")
	}
}
