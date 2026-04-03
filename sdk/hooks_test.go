package sdk_test

import (
	"context"
	"testing"

	"go.edgescale.dev/kernel/sdk"
)

func TestHookRegistry_BeforeAfter(t *testing.T) {
	reg := sdk.NewHookRegistry()
	var callOrder []string

	reg.Before("before.orders.create", func(ctx context.Context, payload any) error {
		callOrder = append(callOrder, "before1")
		return nil
	})
	reg.Before("before.orders.create", func(ctx context.Context, payload any) error {
		callOrder = append(callOrder, "before2")
		return nil
	})
	reg.After("after.orders.create", func(ctx context.Context, payload any) error {
		callOrder = append(callOrder, "after1")
		return nil
	})

	ctx := context.Background()

	if err := reg.FireBefore(ctx, "before.orders.create", nil); err != nil {
		t.Errorf("FireBefore unexpected error: %v", err)
	}
	if err := reg.FireAfter(ctx, "after.orders.create", nil); err != nil {
		t.Errorf("FireAfter unexpected error: %v", err)
	}

	if len(callOrder) != 3 {
		t.Fatalf("Expected 3 calls, got %d", len(callOrder))
	}
	if callOrder[0] != "before1" || callOrder[1] != "before2" || callOrder[2] != "after1" {
		t.Errorf("Call order = %v, want [before1, before2, after1]", callOrder)
	}
}

func TestHookRegistry_BeforeAbort(t *testing.T) {
	reg := sdk.NewHookRegistry()

	reg.Before("before.orders.delete", func(ctx context.Context, payload any) error {
		return sdk.Abort(sdk.Forbidden("cannot delete finalized order"))
	})

	err := reg.FireBefore(context.Background(), "before.orders.delete", nil)
	if err == nil {
		t.Fatal("FireBefore should return error when hook aborts")
	}
}

func TestHookRegistry_NoHandlers(t *testing.T) {
	reg := sdk.NewHookRegistry()

	// Firing on a point with no handlers should succeed
	if err := reg.FireBefore(context.Background(), "before.nonexistent.action", nil); err != nil {
		t.Errorf("FireBefore with no handlers should return nil, got: %v", err)
	}
	if err := reg.FireAfter(context.Background(), "after.nonexistent.action", nil); err != nil {
		t.Errorf("FireAfter with no handlers should return nil, got: %v", err)
	}
}

func TestHookPointHelpers(t *testing.T) {
	before := sdk.BeforeHookPoint("orders", "create")
	if before != "before.orders.create" {
		t.Errorf("BeforeHookPoint = %q, want %q", before, "before.orders.create")
	}

	after := sdk.AfterHookPoint("orders", "create")
	if after != "after.orders.create" {
		t.Errorf("AfterHookPoint = %q, want %q", after, "after.orders.create")
	}
}

func TestReaderRegistry(t *testing.T) {
	reg := sdk.NewReaderRegistry()

	type OrderReader struct {
		Name string
	}

	reg.Register("orders", &OrderReader{Name: "order-reader"})

	reader, err := sdk.GetReader[*OrderReader](reg, "orders")
	if err != nil {
		t.Fatalf("GetReader unexpected error: %v", err)
	}
	if reader.Name != "order-reader" {
		t.Errorf("GetReader.Name = %q, want %q", reader.Name, "order-reader")
	}
}

func TestReaderRegistry_ErrorOnMissing(t *testing.T) {
	reg := sdk.NewReaderRegistry()

	type FakeReader struct{}
	_, err := sdk.GetReader[*FakeReader](reg, "nonexistent")
	if err == nil {
		t.Error("GetReader should return error when reader is not registered")
	}
}
