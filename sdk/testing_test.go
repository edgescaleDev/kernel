package sdk

import (
	"context"
	"testing"
)

func TestNewTestContext(t *testing.T) {
	ctx := NewTestContext("orders")

	if ctx.ServiceID != "orders" {
		t.Errorf("ServiceID = %q, want %q", ctx.ServiceID, "orders")
	}
	if ctx.Logger == nil {
		t.Error("Logger should not be nil")
	}
	if ctx.Bus == nil {
		t.Error("Bus should not be nil")
	}
	if ctx.Hooks == nil {
		t.Error("Hooks should not be nil")
	}
	if ctx.Audit == nil {
		t.Error("Audit should not be nil")
	}
	if ctx.Tasks == nil {
		t.Error("Tasks should not be nil")
	}
	if ctx.Search == nil {
		t.Error("Search should not be nil")
	}
}

func TestTestBus_RecordsEvents(t *testing.T) {
	bus := &TestBus{}
	ctx := context.Background()

	bus.Publish(ctx, "orders.created", map[string]string{"id": "123"})
	bus.Publish(ctx, "orders.updated", map[string]string{"id": "456"})

	events := bus.Events()
	if len(events) != 2 {
		t.Fatalf("events count = %d, want 2", len(events))
	}
	if events[0].Subject != "orders.created" {
		t.Errorf("events[0] subject = %q, want %q", events[0].Subject, "orders.created")
	}
	if events[1].Subject != "orders.updated" {
		t.Errorf("events[1] subject = %q, want %q", events[1].Subject, "orders.updated")
	}
}

func TestTestBus_Reset(t *testing.T) {
	bus := &TestBus{}
	ctx := context.Background()

	bus.Publish(ctx, "test.event", nil)
	bus.Reset()

	if len(bus.Events()) != 0 {
		t.Error("Reset should clear all events")
	}
}

func TestTestAuditLogger_RecordsEntries(t *testing.T) {
	logger := &TestAuditLogger{}
	ctx := context.Background()

	logger.Log(ctx, AuditEntry{Action: AuditCreate, Resource: "order", ResourceID: "123"})

	entries := logger.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries count = %d, want 1", len(entries))
	}
	if entries[0].Resource != "order" {
		t.Errorf("entry resource = %q, want %q", entries[0].Resource, "order")
	}
}

func TestTestTaskExecutor_RecordsTasks(t *testing.T) {
	executor := &TestTaskExecutor{}
	ctx := context.Background()

	opID, err := executor.Execute(ctx, TaskDefinition{Name: "import_csv"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if opID == "" {
		t.Error("operation ID should not be empty")
	}

	tasks := executor.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("tasks count = %d, want 1", len(tasks))
	}
	if tasks[0].Name != "import_csv" {
		t.Errorf("task name = %q, want %q", tasks[0].Name, "import_csv")
	}
}

func TestSoftDeletable_IsDeleted(t *testing.T) {
	s := SoftDeletable{}
	if s.IsDeleted() {
		t.Error("new SoftDeletable should not be deleted")
	}
}
