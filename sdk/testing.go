package sdk

import (
	"context"
	"log/slog"
	"sync"

	"github.com/google/uuid"
)

// TestContext creates a minimal sdk.Context for use in unit tests.
// Uses in-memory fakes for all infrastructure dependencies.
//
// Usage:
//
//	ctx := sdk.NewTestContext("orders")
//	ctx.Bus.Publish(ctx, "orders.created", payload)
//	events := ctx.Bus.(*sdk.TestBus).Events()
func NewTestContext(moduleID string) *Context {
	bus := &TestBus{}
	return &Context{
		Logger:    slog.Default().With("module", moduleID),
		Bus:       bus,
		Hooks:     NewHookRegistry(),
		Audit:     &TestAuditLogger{},
		Tasks:     &TestTaskExecutor{},
		Search:    &TestSearchEngine{},
		readers:   NewReaderRegistry(),
		ServiceID: moduleID,
	}
}

// TestBus is an in-memory EventBus that records published events for assertions.
type TestBus struct {
	mu     sync.Mutex
	events []TestEvent
}

// TestEvent records a single published event.
type TestEvent struct {
	Subject string
	Payload any
}

func (b *TestBus) Publish(_ context.Context, subject string, payload any) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, TestEvent{Subject: subject, Payload: payload})
	return nil
}

func (b *TestBus) Subscribe(_ string, _ string, _ EventHandler) error {
	return nil
}

// Events returns all published events. Thread-safe.
func (b *TestBus) Events() []TestEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	copied := make([]TestEvent, len(b.events))
	copy(copied, b.events)
	return copied
}

// Reset clears all recorded events.
func (b *TestBus) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = nil
}

// TestAuditLogger records audit log entries in memory.
type TestAuditLogger struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func (l *TestAuditLogger) Log(_ context.Context, entry AuditEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
	return nil
}

// Entries returns all logged audit entries. Thread-safe.
func (l *TestAuditLogger) Entries() []AuditEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	copied := make([]AuditEntry, len(l.entries))
	copy(copied, l.entries)
	return copied
}

// TestTaskExecutor records submitted tasks in memory.
type TestTaskExecutor struct {
	mu    sync.Mutex
	tasks []TaskDefinition
}

func (e *TestTaskExecutor) Execute(_ context.Context, task TaskDefinition) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tasks = append(e.tasks, task)
	return uuid.New().String(), nil
}

func (e *TestTaskExecutor) Cancel(_ context.Context, _ string) error {
	return nil
}

// Tasks returns all submitted task definitions. Thread-safe.
func (e *TestTaskExecutor) Tasks() []TaskDefinition {
	e.mu.Lock()
	defer e.mu.Unlock()
	copied := make([]TaskDefinition, len(e.tasks))
	copy(copied, e.tasks)
	return copied
}

// TestSearchEngine is a no-op search engine for tests.
type TestSearchEngine struct{}

func (s *TestSearchEngine) CreateIndex(_ context.Context, _ string, _ IndexSettings) error {
	return nil
}
func (s *TestSearchEngine) Index(_ context.Context, _ string, _ string, _ any) error { return nil }
func (s *TestSearchEngine) BatchIndex(_ context.Context, _ string, _ []SearchDocument) error {
	return nil
}
func (s *TestSearchEngine) Search(_ context.Context, _ string, _ SearchQuery) (*SearchResult, error) {
	return &SearchResult{}, nil
}
func (s *TestSearchEngine) Delete(_ context.Context, _ string, _ string) error { return nil }
func (s *TestSearchEngine) DeleteIndex(_ context.Context, _ string) error       { return nil }
