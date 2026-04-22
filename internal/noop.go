package internal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.edgescale.dev/kernel/sdk"
)

// NoopIdentityProvider rejects all authentication attempts.
// Used when no IdentityProvider is configured - fail-closed by default.
type NoopIdentityProvider struct{}

func (NoopIdentityProvider) Authenticate(_ context.Context, _ http.Header) (*sdk.Identity, error) {
	return nil, errors.New("no identity provider configured")
}

// NoopEventBus discards all published events and accepts all subscriptions.
// Used when no EventBus implementation is provided.
type NoopEventBus struct{}

func (NoopEventBus) Publish(_ context.Context, _ string, _ any) error       { return nil }
func (NoopEventBus) Subscribe(_ string, _ string, _ sdk.EventHandler) error { return nil }

// NoopTaskExecutor silently ignores all task submissions.
// Used when no TaskExecutor implementation is provided.
type NoopTaskExecutor struct{}

func (NoopTaskExecutor) Execute(_ context.Context, _ sdk.TaskDefinition) (string, error) {
	return "", nil
}
func (NoopTaskExecutor) Cancel(_ context.Context, _ string) error { return nil }

// NoopSearchEngine silently ignores all index and search operations.
// Used when no SearchEngine implementation is provided.
type NoopSearchEngine struct{}

func (NoopSearchEngine) CreateIndex(_ context.Context, _ string, _ sdk.IndexSettings) error {
	return nil
}
func (NoopSearchEngine) Index(_ context.Context, _ string, _ string, _ any) error { return nil }
func (NoopSearchEngine) BatchIndex(_ context.Context, _ string, _ []sdk.SearchDocument) error {
	return nil
}
func (NoopSearchEngine) Search(_ context.Context, _ string, _ sdk.SearchQuery) (*sdk.SearchResult, error) {
	return &sdk.SearchResult{Hits: []json.RawMessage{}}, nil
}
func (NoopSearchEngine) Delete(_ context.Context, _ string, _ string) error { return nil }
func (NoopSearchEngine) DeleteIndex(_ context.Context, _ string) error      { return nil }
