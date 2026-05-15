package sdk

import (
	"context"
	"testing"
)

// testShiftsClient is a mock client for testing.
type testShiftsClient struct {
	listCalled bool
}

func (c *testShiftsClient) ListShifts(_ context.Context) ([]string, error) {
	c.listCalled = true
	return []string{"morning", "evening"}, nil
}

func TestClient_RegisterAndResolve(t *testing.T) {
	ctx := NewTestContext("shifts")

	mock := &testShiftsClient{}
	ctx.RegisterClient(mock)

	// Resolve via Client[T]
	client, err := Client[*testShiftsClient](ctx, "shifts")
	if err != nil {
		t.Fatalf("Client[T] error: %v", err)
	}

	result, err := client.ListShifts(context.Background())
	if err != nil {
		t.Fatalf("ListShifts error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("result count = %d, want 2", len(result))
	}
	if !client.listCalled {
		t.Error("expected listCalled to be true")
	}
}

func TestClient_BackwardCompatibleWithReader(t *testing.T) {
	ctx := NewTestContext("shifts")

	mock := &testShiftsClient{}
	ctx.RegisterClient(mock)

	// RegisterClient should be discoverable via Reader[T] too.
	client, err := Reader[*testShiftsClient](ctx, "shifts")
	if err != nil {
		t.Fatalf("Reader[T] should resolve a RegisterClient registration: %v", err)
	}
	if client != mock {
		t.Error("Reader[T] should return the same instance registered via RegisterClient")
	}
}

func TestClient_ErrorOnMissing(t *testing.T) {
	ctx := NewTestContext("shifts")

	_, err := Client[*testShiftsClient](ctx, "nonexistent")
	if err == nil {
		t.Error("Client[T] should return an error for unregistered modules")
	}
}

func TestClient_RegisterReaderResolvableViaClient(t *testing.T) {
	ctx := NewTestContext("shifts")

	mock := &testShiftsClient{}
	ctx.RegisterReader(mock)

	// RegisterReader should be discoverable via Client[T] too.
	client, err := Client[*testShiftsClient](ctx, "shifts")
	if err != nil {
		t.Fatalf("Client[T] should resolve a RegisterReader registration: %v", err)
	}
	if client != mock {
		t.Error("Client[T] should return the same instance registered via RegisterReader")
	}
}
