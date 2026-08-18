package model

import (
	"crypto/rand"
	"time"
)

const handleAlpha = "abcdefghijklmnopqrstuvwxyz0123456789"

// Workspace isolates data per tenant.
type Workspace struct {
	ID        string    `json:"id"`
	Handle    string    `json:"handle"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Plan      string    `json:"plan"`
	CreatedAt time.Time `json:"created_at"`
}

// Feed represents an RSS/Atom feed subscription.
type Feed struct {
	ID            string     `json:"id"`
	Handle        string     `json:"handle"`
	WorkspaceID  string     `json:"workspace_id"`
	URL           string     `json:"url"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	SiteURL       string     `json:"site_url"`
	LastRefreshed *time.Time `json:"last_refreshed,omitempty"`
	EntryCount    int        `json:"entry_count"`
	CreatedAt     time.Time  `json:"created_at"`
}

// Entry represents a single item from a feed.
type Entry struct {
	ID          string     `json:"id"`
	Handle      string     `json:"handle"`
	FeedID      string     `json:"feed_id"`
	WorkspaceID string     `json:"workspace_id"`
	GUID        string     `json:"guid"`
	Title       string     `json:"title"`
	Link        string     `json:"link"`
	Summary     string     `json:"summary"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	Read        bool       `json:"read"`
	Starred     bool       `json:"starred"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Token is a long-lived bearer token.
type Token struct {
	ID          string    `json:"id"`
	Handle      string    `json:"handle"`
	WorkspaceID string    `json:"workspace_id"`
	Token       string    `json:"token"`
	CreatedAt   time.Time `json:"created_at"`
}

// OTP is a one-time password for auth.
type OTP struct {
	Email     string    `json:"email"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AuditLog records an action.
type AuditLog struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Action      string    `json:"action"`
	Detail      string    `json:"detail"`
	At          time.Time `json:"at"`
}

// GenerateHandle creates a short stable handle like "feed_a1b2c".
func GenerateHandle(prefix string) string {
	return prefix + "_" + randStr(5)
}

// GenerateID creates a unique ID.
func GenerateID() string {
	return randStr(16)
}

// GenerateToken creates a long random token string.
func GenerateToken() string {
	return randStr(32)
}

func randStr(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = handleAlpha[int(b[i])%len(handleAlpha)]
	}
	return string(b)
}
