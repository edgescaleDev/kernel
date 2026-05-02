package sdk_test

import (
	"testing"

	"github.com/edgescaleDev/kernel/sdk"
)

func TestServiceType_String(t *testing.T) {
	tests := []struct {
		stype sdk.ModuleType
		want  string
	}{
		{sdk.TypeCore, "core"},
		{sdk.TypeFeature, "feature"},
		{sdk.TypeIntegration, "integration"},
		{sdk.ModuleType(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.stype.String(); got != tt.want {
			t.Errorf("ServiceType(%d).String() = %q, want %q", tt.stype, got, tt.want)
		}
	}
}

func TestManifest_Fields(t *testing.T) {
	m := sdk.Manifest{
		ID:          "billing",
		Name:        "Billing & Invoicing",
		Version:     "1.0.0",
		Type:        sdk.TypeFeature,
		Schema:      "module_billing",
		Description: "Handles invoices and payments",
		DependsOn:   []string{"iam", "orders"},
		Permissions: []sdk.Permission{
			{Key: "invoices.create", Label: sdk.T("Create invoices")},
			{Key: "invoices.read", Label: sdk.T("View invoices")},
		},
		Config: []sdk.ConfigFieldDef{
			{Key: "auto_approve", Type: "bool", Default: false, Label: sdk.T("Auto-approve invoices"), Required: false},
		},
		UINav: []sdk.NavItem{
			{Label: sdk.T("Invoices"), Icon: "receipt", Path: "/billing/invoices", Permission: "invoices.read", SortOrder: 1},
		},
		StoragePrefix: "billing",
	}

	if m.ID != "billing" {
		t.Errorf("Manifest.ID = %q, want %q", m.ID, "billing")
	}
	if m.Type != sdk.TypeFeature {
		t.Errorf("Manifest.Type = %v, want TypeFeature", m.Type)
	}
	if len(m.DependsOn) != 2 {
		t.Errorf("Manifest.DependsOn length = %d, want 2", len(m.DependsOn))
	}
	if len(m.Permissions) != 2 {
		t.Errorf("Manifest.Permissions length = %d, want 2", len(m.Permissions))
	}
	if len(m.Config) != 1 {
		t.Errorf("Manifest.Config length = %d, want 1", len(m.Config))
	}
	if m.StoragePrefix != "billing" {
		t.Errorf("Manifest.StoragePrefix = %q, want %q", m.StoragePrefix, "billing")
	}
}
