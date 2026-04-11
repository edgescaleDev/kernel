package kernel

import (
	"go.edgescale.dev/kernel/internal"
)

// healthChecker is implemented by modules that want to report their own health.
type healthChecker = internal.HealthChecker

// healthStatus represents a module's health state.
type healthStatus = internal.HealthStatus
