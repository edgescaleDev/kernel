package iam

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const invitationTTL = 7 * 24 * time.Hour // 7 days

// generateSecureToken creates a cryptographically random URL-safe token.
func generateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// defaultExpiresAt returns the default invitation expiry timestamp.
func defaultExpiresAt() time.Time {
	return time.Now().Add(invitationTTL)
}

// orgID extracts the org_id from the gin context as uuid.UUID.
// The kernel's resolveOrg() middleware stores it as uuid.UUID.
func orgID(c *gin.Context) uuid.UUID {
	v, _ := c.Get("org_id")
	id, _ := v.(uuid.UUID)
	return id
}

// userSubject extracts the authenticated user's IdP subject from context.
// Set by the kernel's authenticate() middleware as identity.Subject.
func userSubject(c *gin.Context) string {
	return c.GetString("user_id")
}
