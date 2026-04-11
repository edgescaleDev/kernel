package kernel

// HealthChecker is implemented by modules that want to report their own health.
type HealthChecker interface {
	HealthCheck() HealthStatus
}

// HealthStatus represents a module's health state.
type HealthStatus struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message,omitempty"`
}

func statusString(healthy bool) string {
	if healthy {
		return "healthy"
	}
	return "degraded"
}
