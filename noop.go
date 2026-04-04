package kernel

import (
	"context"
	"encoding/json"
	"errors"

	"go.edgescale.dev/kernel/sdk"
)

// noopIdentityProvider rejects all authentication attempts.
// Used when no IdentityProvider is configured — fail-closed by default.
type noopIdentityProvider struct{}

func (noopIdentityProvider) ValidateToken(_ context.Context, _ string) (*sdk.Identity, error) {
	return nil, errors.New("no identity provider configured")
}
func (noopIdentityProvider) RevokeToken(_ context.Context, _ string) error {
	return errors.New("no identity provider configured")
}

// noopEventBus discards all published events and accepts all subscriptions.
// Used when no EventBus implementation is provided.
type noopEventBus struct{}

func (noopEventBus) Publish(_ context.Context, _ string, _ any) error       { return nil }
func (noopEventBus) Subscribe(_ string, _ string, _ sdk.EventHandler) error { return nil }

// noopTaskExecutor silently ignores all task submissions.
// Used when no TaskExecutor implementation is provided.
type noopTaskExecutor struct{}

func (noopTaskExecutor) Execute(_ context.Context, _ sdk.TaskDefinition) (string, error) {
	return "", nil
}
func (noopTaskExecutor) Cancel(_ context.Context, _ string) error { return nil }

// noopSearchEngine silently ignores all index and search operations.
// Used when no SearchEngine implementation is provided.
type noopSearchEngine struct{}

func (noopSearchEngine) CreateIndex(_ context.Context, _ string, _ sdk.IndexSettings) error {
	return nil
}
func (noopSearchEngine) Index(_ context.Context, _ string, _ string, _ any) error { return nil }
func (noopSearchEngine) BatchIndex(_ context.Context, _ string, _ []sdk.SearchDocument) error {
	return nil
}
func (noopSearchEngine) Search(_ context.Context, _ string, _ sdk.SearchQuery) (*sdk.SearchResult, error) {
	return &sdk.SearchResult{Hits: []json.RawMessage{}}, nil
}
func (noopSearchEngine) Delete(_ context.Context, _ string, _ string) error { return nil }
func (noopSearchEngine) DeleteIndex(_ context.Context, _ string) error      { return nil }
