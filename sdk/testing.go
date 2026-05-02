package sdk

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TestContext creates a minimal sdk.Context for use in unit tests.
// Uses in-memory fakes for all infrastructure dependencies.
//
// Usage:
//
//	ctx := sdk.NewTestContext("orders")
//	ctx.Bus.Publish(ctx, "orders.created", payload)
//	events := ctx.Bus.(*sdk.TestBus).Events()
func NewTestContext(moduleID string) *Context {
	bus := &TestBus{}
	return &Context{
		Logger:    slog.Default().With("module", moduleID),
		Bus:       bus,
		Hooks:     NewHookRegistry(),
		Audit:     &TestAuditLogger{},
		Tasks:     &TestTaskExecutor{},
		Search:    &TestSearchEngine{},
		Storage:   &TestObjectStore{},
		readers:   NewReaderRegistry(),
		ServiceID: moduleID,
	}
}

// TestBus is an in-memory EventBus that records published events for assertions.
type TestBus struct {
	mu     sync.Mutex
	events []TestEvent
}

// TestEvent records a single published event.
type TestEvent struct {
	Subject string
	Payload any
}

func (b *TestBus) Publish(_ context.Context, subject string, payload any) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, TestEvent{Subject: subject, Payload: payload})
	return nil
}

func (b *TestBus) Subscribe(_ string, _ string, _ EventHandler) error {
	return nil
}

// Events returns all published events. Thread-safe.
func (b *TestBus) Events() []TestEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	copied := make([]TestEvent, len(b.events))
	copy(copied, b.events)
	return copied
}

// Reset clears all recorded events.
func (b *TestBus) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = nil
}

// TestAuditLogger records audit log entries in memory.
type TestAuditLogger struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func (l *TestAuditLogger) Log(_ context.Context, entry AuditEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
	return nil
}

// Entries returns all logged audit entries. Thread-safe.
func (l *TestAuditLogger) Entries() []AuditEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	copied := make([]AuditEntry, len(l.entries))
	copy(copied, l.entries)
	return copied
}

// TestTaskExecutor records submitted tasks in memory.
type TestTaskExecutor struct {
	mu    sync.Mutex
	tasks []TaskDefinition
}

func (e *TestTaskExecutor) Execute(_ context.Context, task TaskDefinition) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tasks = append(e.tasks, task)
	return uuid.New().String(), nil
}

func (e *TestTaskExecutor) Cancel(_ context.Context, _ string) error {
	return nil
}

// Tasks returns all submitted task definitions. Thread-safe.
func (e *TestTaskExecutor) Tasks() []TaskDefinition {
	e.mu.Lock()
	defer e.mu.Unlock()
	copied := make([]TaskDefinition, len(e.tasks))
	copy(copied, e.tasks)
	return copied
}

// TestSearchEngine is a no-op search engine for tests.
type TestSearchEngine struct{}

func (s *TestSearchEngine) CreateIndex(_ context.Context, _ string, _ IndexSettings) error {
	return nil
}
func (s *TestSearchEngine) Index(_ context.Context, _ string, _ string, _ any) error { return nil }
func (s *TestSearchEngine) BatchIndex(_ context.Context, _ string, _ []SearchDocument) error {
	return nil
}
func (s *TestSearchEngine) Search(_ context.Context, _ string, _ SearchQuery) (*SearchResult, error) {
	return &SearchResult{}, nil
}
func (s *TestSearchEngine) Delete(_ context.Context, _ string, _ string) error { return nil }
func (s *TestSearchEngine) DeleteIndex(_ context.Context, _ string) error      { return nil }

// TestLockProvider is an in-memory lock provider for tests.
// In single-process tests, locks are always acquired successfully.
type TestLockProvider struct {
	mu    sync.Mutex
	locks map[string]bool
}

func (p *TestLockProvider) Acquire(_ context.Context, key string, _ time.Duration) (func(), bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.locks == nil {
		p.locks = make(map[string]bool)
	}
	if p.locks[key] {
		return nil, false, nil
	}
	p.locks[key] = true
	release := func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.locks, key)
	}
	return release, true, nil
}

// TestObjectStore is an in-memory object store for tests.
// It implements both ObjectStore and Uploader for full test coverage.
type TestObjectStore struct {
	mu      sync.Mutex
	objects map[string]testObject // key: "bucket/key"
}

type testObject struct {
	data        []byte
	contentType string
	metadata    map[string]string
	modifiedAt  time.Time
}

func (s *TestObjectStore) init() {
	if s.objects == nil {
		s.objects = make(map[string]testObject)
	}
}

func (s *TestObjectStore) PresignURL(_ context.Context, input PresignInput) (*PresignResult, error) {
	return &PresignResult{
		URL:       "https://test-storage.example.com/" + input.Bucket + "/" + input.Key,
		Method:    input.Method,
		ExpiresAt: time.Now().Add(input.Expiry),
	}, nil
}

func (s *TestObjectStore) PublicURL(_ context.Context, bucket string, key string) string {
	return "https://test-cdn.example.com/" + bucket + "/" + key
}

func (s *TestObjectStore) Delete(_ context.Context, bucket string, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	delete(s.objects, bucket+"/"+key)
	return nil
}

func (s *TestObjectStore) Head(_ context.Context, bucket string, key string) (*ObjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	obj, ok := s.objects[bucket+"/"+key]
	if !ok {
		return nil, NotFound("object", key)
	}
	return &ObjectInfo{
		Bucket:       bucket,
		Key:          key,
		Size:         int64(len(obj.data)),
		ContentType:  obj.contentType,
		LastModified: obj.modifiedAt,
		Metadata:     obj.metadata,
	}, nil
}

func (s *TestObjectStore) Upload(_ context.Context, input UploadInput) (*ObjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	data, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	s.objects[input.Bucket+"/"+input.Key] = testObject{
		data:        data,
		contentType: input.ContentType,
		metadata:    input.Metadata,
		modifiedAt:  now,
	}
	return &ObjectInfo{
		Bucket:       input.Bucket,
		Key:          input.Key,
		Size:         int64(len(data)),
		ContentType:  input.ContentType,
		LastModified: now,
		Metadata:     input.Metadata,
	}, nil
}

func (s *TestObjectStore) Download(_ context.Context, bucket string, key string) (*ObjectReader, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	obj, ok := s.objects[bucket+"/"+key]
	if !ok {
		return nil, NotFound("object", key)
	}
	return &ObjectReader{
		Body: io.NopCloser(bytes.NewReader(obj.data)),
		Info: ObjectInfo{
			Bucket:       bucket,
			Key:          key,
			Size:         int64(len(obj.data)),
			ContentType:  obj.contentType,
			LastModified: obj.modifiedAt,
			Metadata:     obj.metadata,
		},
	}, nil
}

// Objects returns all stored object keys. Thread-safe.
func (s *TestObjectStore) Objects() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		keys = append(keys, k)
	}
	return keys
}
