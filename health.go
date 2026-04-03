package kernel

import (
	"context"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// HealthChecker is implemented by modules that want to report their own health.
type HealthChecker interface {
	HealthCheck() HealthStatus
}

// HealthStatus represents a module's health state.
type HealthStatus struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message,omitempty"`
}

// handleDetailedHealth returns per-module health for internal dashboards.
// GET /v1/health (authenticated)
func (k *Kernel) handleDetailedHealth(c *gin.Context) {
	result := make(map[string]HealthStatus)
	var unhealthy bool
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, m := range k.Modules() {
		checker, ok := m.(HealthChecker)
		if !ok {
			continue
		}

		manifest := m.Manifest()
		wg.Add(1)
		go func(id string, hc HealthChecker) {
			defer wg.Done()
			status := hc.HealthCheck()
			mu.Lock()
			result[id] = status
			if !status.Healthy {
				unhealthy = true
			}
			mu.Unlock()
		}(manifest.ID, checker)
	}

	wg.Wait()

	// Add infrastructure health.
	result["database"] = k.checkDatabaseHealth()
	result["redis"] = k.checkRedisHealth()

	status := http.StatusOK
	if unhealthy || !result["database"].Healthy || !result["redis"].Healthy {
		status = http.StatusServiceUnavailable
	}

	c.JSON(status, gin.H{
		"status":  statusString(status == http.StatusOK),
		"modules": result,
	})
}

func (k *Kernel) checkDatabaseHealth() HealthStatus {
	if k.db == nil {
		return HealthStatus{Healthy: false, Message: "not configured"}
	}
	sqlDB, err := k.db.DB()
	if err != nil {
		return HealthStatus{Healthy: false, Message: err.Error()}
	}
	if err := sqlDB.Ping(); err != nil {
		return HealthStatus{Healthy: false, Message: err.Error()}
	}
	return HealthStatus{Healthy: true}
}

func (k *Kernel) checkRedisHealth() HealthStatus {
	if k.redis == nil {
		return HealthStatus{Healthy: false, Message: "not configured"}
	}
	if err := k.redis.Ping(context.Background()).Err(); err != nil {
		return HealthStatus{Healthy: false, Message: err.Error()}
	}
	return HealthStatus{Healthy: true}
}

func statusString(healthy bool) string {
	if healthy {
		return "healthy"
	}
	return "degraded"
}
