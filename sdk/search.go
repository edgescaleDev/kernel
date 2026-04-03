package sdk

import (
	"context"
	"encoding/json"
)

// SearchEngine abstracts full-text and faceted search behind a pluggable interface.
// OS consumers choose their search engine (Meilisearch, Elasticsearch, Typesense, etc.).
type SearchEngine interface {
	// CreateIndex creates a new search index with the given settings.
	CreateIndex(ctx context.Context, index string, settings IndexSettings) error

	// Index creates or updates a single document in the search index.
	Index(ctx context.Context, index string, id string, doc any) error

	// BatchIndex indexes multiple documents in a single operation.
	BatchIndex(ctx context.Context, index string, docs []SearchDocument) error

	// Search performs a search query and returns matching results.
	Search(ctx context.Context, index string, query SearchQuery) (*SearchResult, error)

	// Delete removes a document from the search index.
	Delete(ctx context.Context, index string, id string) error

	// DeleteIndex removes an entire search index.
	DeleteIndex(ctx context.Context, index string) error
}

// IndexSettings configures a search index.
type IndexSettings struct {
	// PrimaryKey is the field used as the document identifier.
	PrimaryKey string

	// SearchableAttributes lists fields that are searchable.
	SearchableAttributes []string

	// FilterableAttributes lists fields that can be used in filters.
	FilterableAttributes []string

	// SortableAttributes lists fields that can be used for sorting.
	SortableAttributes []string
}

// SearchDocument represents a document to be indexed.
type SearchDocument struct {
	// ID is the document identifier.
	ID string

	// Body is the document content to index.
	Body any
}

// SearchQuery describes a search request.
type SearchQuery struct {
	// Query is the search text.
	Query string

	// Filters are key-value pairs for filtering results.
	Filters map[string]any

	// Sort specifies the sort order (e.g., ["created_at:desc"]).
	Sort []string

	// Offset is the number of results to skip (for offset-based pagination).
	Offset int

	// Limit is the maximum number of results to return.
	Limit int

	// FacetsBy lists fields to compute facet counts for.
	FacetsBy []string
}

// SearchResult contains the results of a search query.
type SearchResult struct {
	// Hits contains the matching documents as raw JSON.
	Hits []json.RawMessage `json:"hits"`

	// Total is the total number of matching documents.
	Total int `json:"total"`

	// Facets contains facet counts grouped by field.
	Facets map[string][]FacetCount `json:"facets,omitempty"`

	// QueryTimeMs is the search engine's processing time in milliseconds.
	QueryTimeMs int `json:"query_time_ms"`
}

// FacetCount represents a single facet value and its count.
type FacetCount struct {
	// Value is the facet value.
	Value string `json:"value"`

	// Count is the number of documents with this facet value.
	Count int `json:"count"`
}
