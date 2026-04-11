package kernel

import (
	"go.edgescale.dev/kernel/internal"
)

// installFallbacks sets noop implementations for pluggable interfaces
// that were not explicitly set by the consumer. This prevents nil
// dereferences when modules access Tasks, Search, or Bus.
func (k *Kernel) installFallbacks() {
	if k.identityProvider == nil {
		k.logger.Warn("no identity provider set - all authentication will be rejected")
		k.identityProvider = internal.NoopIdentityProvider{}
	}
	if k.bus == nil {
		k.logger.Warn("no event bus set - using noop bus")
		k.bus = internal.NoopEventBus{}
	}
	if k.taskExecutor == nil {
		k.logger.Warn("no task executor set - background tasks will be disabled")
		k.taskExecutor = internal.NoopTaskExecutor{}
	}
	if k.searchEngine == nil {
		k.logger.Warn("no search engine set - search will be disabled")
		k.searchEngine = internal.NoopSearchEngine{}
	}
}
