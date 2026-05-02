package sdk

import (
	"context"
	"io"
	"time"
)

// ObjectStore abstracts blob storage behind a pluggable interface.
// OS consumers choose their storage backend (S3, GCS, MinIO, local filesystem, etc.).
//
// The primary flow is presigned URLs: modules generate time-limited URLs
// and hand them to clients for direct upload/download. The kernel never
// proxies file content in the hot path.
//
// For local/NFS backends, PresignURL returns a signed URL pointing back
// to the kernel, which validates the signature and streams to/from disk.
//
// For server-side file operations (report generation, CSV exports),
// backends may implement the companion Uploader interface.
type ObjectStore interface {
	// PresignURL generates a time-limited URL for direct client access.
	// Method "PUT" for uploads, "GET" for downloads.
	//
	// For cloud backends (S3, GCS), the URL points directly to the provider.
	// For local backends, the URL points back to the kernel with an HMAC signature.
	PresignURL(ctx context.Context, input PresignInput) (*PresignResult, error)

	// PublicURL returns a permanent, non-expiring URL for an object in a
	// public-read bucket. Use for assets that need stable, cacheable URLs
	// (avatars, logos, product images).
	//
	// The returned URL depends on the backend configuration:
	//   R2/S3 with CDN: https://cdn.example.com/key
	//   GCS:            https://storage.googleapis.com/bucket/key
	//   Local:          https://api.example.com/_storage/public/key
	//
	// Calling this on a private bucket will return a URL that results in 403.
	PublicURL(ctx context.Context, bucket string, key string) string

	// Delete removes an object from storage.
	Delete(ctx context.Context, bucket string, key string) error

	// Head returns object metadata without downloading the content.
	// Returns a NotFound error if the object does not exist.
	Head(ctx context.Context, bucket string, key string) (*ObjectInfo, error)
}

// Uploader is a companion interface for backends that support server-side
// file operations. Cloud backends (S3, GCS, MinIO) and local backends
// all implement this, but it is not required by the primary ObjectStore contract.
//
// Use case: background tasks that generate files (PDF reports, CSV exports),
// thumbnail generation, or internal data migration.
//
// Detect support via type assertion:
//
//	if uploader, ok := m.ctx.Storage.(sdk.Uploader); ok {
//	    info, err := uploader.Upload(ctx, input)
//	}
type Uploader interface {
	// Upload stores an object from an io.Reader and returns its metadata.
	Upload(ctx context.Context, input UploadInput) (*ObjectInfo, error)

	// Download retrieves an object's content.
	// The caller MUST close the returned ObjectReader.Body when done.
	Download(ctx context.Context, bucket string, key string) (*ObjectReader, error)
}

// PresignInput describes a presigned URL request.
type PresignInput struct {
	// Bucket is the storage bucket/container name.
	Bucket string

	// Key is the object path within the bucket.
	Key string

	// Method is the HTTP method: "GET" for download, "PUT" for upload.
	Method string

	// Expiry is how long the URL stays valid.
	Expiry time.Duration

	// ContentType is required for PUT presigning to enforce the upload MIME type.
	ContentType string
}

// PresignResult contains the generated presigned URL.
type PresignResult struct {
	// URL is the presigned URL for direct access.
	URL string `json:"url"`

	// Method is the HTTP method this URL is valid for.
	Method string `json:"method"`

	// ExpiresAt is when the URL expires.
	ExpiresAt time.Time `json:"expires_at"`

	// Headers are additional headers the client must send with the request.
	// For PUT presigning, this typically includes Content-Type.
	Headers map[string]string `json:"headers,omitempty"`
}

// UploadInput describes an object to store via the Uploader companion interface.
type UploadInput struct {
	// Bucket is the storage bucket/container name.
	Bucket string

	// Key is the object path within the bucket.
	Key string

	// Body is the object content to upload.
	Body io.Reader

	// ContentType is the MIME type (e.g., "application/pdf", "image/png").
	ContentType string

	// Size is the content length in bytes. -1 if unknown.
	Size int64

	// Metadata is optional key-value pairs stored alongside the object.
	Metadata map[string]string
}

// ObjectInfo contains metadata about a stored object.
type ObjectInfo struct {
	// Bucket is the storage bucket name.
	Bucket string `json:"bucket"`

	// Key is the object path within the bucket.
	Key string `json:"key"`

	// Size is the object size in bytes.
	Size int64 `json:"size"`

	// ContentType is the MIME type of the object.
	ContentType string `json:"content_type"`

	// ETag is the entity tag (content hash) for cache validation.
	ETag string `json:"etag,omitempty"`

	// LastModified is when the object was last updated.
	LastModified time.Time `json:"last_modified"`

	// Metadata contains custom key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ObjectReader wraps the downloaded object content from the Uploader interface.
type ObjectReader struct {
	// Body is the object content. Caller MUST close it.
	Body io.ReadCloser

	// Info contains the object metadata.
	Info ObjectInfo
}
