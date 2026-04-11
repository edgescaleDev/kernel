package sdk

import (
	"context"
)

// FeatureFlags checks whether feature flags are active for an organization.
// The kernel provides a noop implementation that returns false for all flags
// when no feature flags module is registered.
type FeatureFlags interface {
	// Enabled checks whether a feature flag is active for the given org.
	Enabled(ctx context.Context, flag string, orgID string) bool
}

// noopFeatureFlags is the default implementation when no feature flags module is registered.
type noopFeatureFlags struct{}

func (noopFeatureFlags) Enabled(_ context.Context, _ string, _ string) bool { return false }

// NoopFeatureFlags returns a FeatureFlags implementation that always returns false.
func NoopFeatureFlags() FeatureFlags { return noopFeatureFlags{} }
