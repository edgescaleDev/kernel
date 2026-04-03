package sdk

import (
	"fmt"
	"sync"
)

// ReaderRegistry provides type-safe cross-service data access.
// Services register their reader implementations during Init(),
// and other services retrieve them via GetReader[T]().
type ReaderRegistry struct {
	mu      sync.RWMutex
	readers map[string]any
}

// NewReaderRegistry creates a new empty ReaderRegistry.
func NewReaderRegistry() *ReaderRegistry {
	return &ReaderRegistry{
		readers: make(map[string]any),
	}
}

// Register stores a reader implementation under the service ID.
// Called during service Init() to expose cross-service read APIs.
func (r *ReaderRegistry) Register(serviceID string, reader any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readers[serviceID] = reader
}

// GetReader retrieves a type-safe reader for the given service.
// Returns an error if the service has no registered reader or the type assertion fails.
func GetReader[T any](reg *ReaderRegistry, serviceID string) (T, error) {
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	var zero T

	reader, ok := reg.readers[serviceID]
	if !ok {
		return zero, fmt.Errorf("sdk: no reader registered for module %q", serviceID)
	}

	typed, ok := reader.(T)
	if !ok {
		return zero, fmt.Errorf("sdk: reader for module %q is not of expected type %T", serviceID, (*T)(nil))
	}
	return typed, nil
}
