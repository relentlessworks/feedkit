package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/relentlessworks/feedkit/internal/auth"
	"github.com/relentlessworks/feedkit/internal/feedparser"
	"github.com/relentlessworks/feedkit/internal/model"
	"github.com/relentlessworks/feedkit/internal/store"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	store *store.Store
	auth  *auth.Auth
}

// New creates a new Handler.
func New(s *store.Store, a *auth.Auth) *Handler {
	return &Handler{store: s, auth: a}
}

// Routes returns the HTTP mux with all routes registered.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/help", h.handleHelp)
	mux.HandleFunc("/.well-known/agent.md", h.handleHelp)
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/auth/request", h.handleAuthRequest)
	mux.HandleFunc("/auth/verify", h.handleAuthVerify)
	mux.HandleFunc("/workspaces", h.handleWorkspaces)
	mux.HandleFunc("/feeds", h.handleFeeds)
	mux.HandleFunc("/feeds/", h.handleFeedDetail)
	mux.HandleFunc("/entries", h.handleEntries)
	mux.HandleFunc("/entries/", h.handleEntryDetail)
	mux.HandleFunc("/audit", h.handleAudit)
	mux.HandleFunc("/mcp", h.handleMCP)
	mux.HandleFunc("/", h.handleRoot)
	return corsMiddleware(h.authMiddleware(mux))
}

func (h *Handler) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		writeOK(w, r, "feedkit running. GET /help for usage.")
		return
	}
	writeError(w, r, http.StatusNotFound, "unknown endpoint: "+r.URL.Path, "GET /help for available endpoints")
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeOK(w, r, "healthy")
}

func (h *Handler) handleHelp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, helpText)
}

func (h *Handler) handleAuthRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
		return
	}
	email := r.FormValue("email")
	if email == "" {
		writeError(w, r, http.StatusBadRequest, "missing email", "provide email field: email=user@example.com")
		return
	}
	code, err := h.auth.RequestOTP(email)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to send OTP", "try again")
		return
	}
	// In dev mode, return the code directly
	writeOK(w, r, fmt.Sprintf("OTP sent to %s (code: %s in dev mode)", email, code))
}

func (h *Handler) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
		return
	}
	email := r.FormValue("email")
	code := r.FormValue("code")
	if email == "" || code == "" {
		writeError(w, r, http.StatusBadRequest, "missing email or code", "provide email and code fields")
		return
	}
	tok, ws, err := h.auth.VerifyOTP(email, code)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, err.Error(), "request a new OTP via POST /auth/request")
		return
	}
	writeRecord(w, r, []string{
		"token=" + tok.Token,
		"workspace=" + ws.Handle,
		"email=" + ws.Email,
	})
}

func (h *Handler) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	ws := getWorkspace(r)
	switch r.Method {
	case http.MethodGet:
		writeRecord(w, r, []string{
			"handle=" + ws.Handle,
			"name=" + ws.Name,
			"email=" + ws.Email,
			"plan=" + ws.Plan,
			"created=" + ws.CreatedAt.Format(time.RFC3339),
		})
	case http.MethodPost:
		name := r.FormValue("name")
		if name != "" {
			ws.Name = name
			_ = h.store.CreateWorkspace(ws)
		}
		writeRecord(w, r, []string{
			"handle=" + ws.Handle,
			"name=" + ws.Name,
			"email=" + ws.Email,
			"plan=" + ws.Plan,
		})
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET or POST")
	}
}

func (h *Handler) handleFeeds(w http.ResponseWriter, r *http.Request) {
	ws := getWorkspace(r)
	switch r.Method {
	case http.MethodGet:
		feeds := h.store.ListFeeds(ws.ID)
		var records [][]string
		for _, f := range feeds {
			records = append(records, feedFields(f))
		}
		if len(records) == 0 {
			writeOK(w, r, "no feeds found")
			return
		}
		writeRecords(w, r, records)
	case http.MethodPost:
		url := r.FormValue("url")
		if url == "" {
			writeError(w, r, http.StatusBadRequest, "missing url", "provide url field: url=https://example.com/feed.xml")
			return
		}
		// Check for duplicate
		existing, _ := h.store.GetFeedByURL(url, ws.ID)
		if existing != nil {
			writeError(w, r, http.StatusConflict, "feed already exists", "use the existing handle: "+existing.Handle)
			return
		}
		// Fetch and parse the feed
		parsed, err := feedparser.FetchAndParse(url)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "failed to fetch/parse feed: "+err.Error(), "check the URL is a valid RSS or Atom feed")
			return
		}
		feed := &model.Feed{
			ID:           model.GenerateID(),
			Handle:       model.GenerateHandle("feed"),
			WorkspaceID:  ws.ID,
			URL:          url,
			Title:        parsed.Title,
			Description:  parsed.Description,
			SiteURL:      parsed.SiteURL,
			EntryCount:   len(parsed.Entries),
			CreatedAt:    time.Now(),
		}
		now := time.Now()
		feed.LastRefreshed = &now
		if err := h.store.CreateFeed(feed); err != nil {
			writeError(w, r, http.StatusInternalServerError, "failed to create feed", "try again")
			return
		}
		// Save entries
		for _, pe := range parsed.Entries {
			entry := &model.Entry{
				ID:          model.GenerateID(),
				Handle:      model.GenerateHandle("entry"),
				FeedID:      feed.ID,
				WorkspaceID: ws.ID,
				GUID:        pe.GUID,
				Title:       pe.Title,
				Link:        pe.Link,
				Summary:     pe.Summary,
				PublishedAt: pe.PublishedAt,
				CreatedAt:   time.Now(),
			}
			_ = h.store.CreateEntry(entry)
		}
		h.store.AddAuditLog(model.AuditLog{
			ID:          model.GenerateID(),
			WorkspaceID: ws.ID,
			Action:      "feed.create",
			Detail:      "url=" + url,
			At:          time.Now(),
		})
		writeRecord(w, r, feedFields(feed))
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET or POST")
	}
}

func (h *Handler) handleFeedDetail(w http.ResponseWriter, r *http.Request) {
	ws := getWorkspace(r)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/feeds/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, r, http.StatusBadRequest, "missing feed handle", "provide feed handle in URL: /feeds/feed_xxxxx")
		return
	}
	handle := parts[0]
	feed, err := h.store.GetFeed(handle, ws.ID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "feed not found", "check the handle with GET /feeds")
		return
	}
	if len(parts) >= 2 && parts[1] == "refresh" {
		if r.Method != http.MethodPost {
			writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
			return
		}
		parsed, err := feedparser.FetchAndParse(feed.URL)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "failed to refresh feed: "+err.Error(), "check if the feed URL is still valid")
			return
		}
		// Update feed metadata
		feed.Title = parsed.Title
		feed.Description = parsed.Description
		feed.SiteURL = parsed.SiteURL
		now := time.Now()
		feed.LastRefreshed = &now
		// Add new entries
		newCount := 0
		for _, pe := range parsed.Entries {
			existing, _ := h.store.GetEntryByGUID(pe.GUID, feed.ID)
			if existing != nil {
				continue
			}
			entry := &model.Entry{
				ID:          model.GenerateID(),
				Handle:      model.GenerateHandle("entry"),
				FeedID:      feed.ID,
				WorkspaceID: ws.ID,
				GUID:        pe.GUID,
				Title:       pe.Title,
				Link:        pe.Link,
				Summary:     pe.Summary,
				PublishedAt: pe.PublishedAt,
				CreatedAt:   time.Now(),
			}
			_ = h.store.CreateEntry(entry)
			newCount++
		}
		// Recount entries
		entries := h.store.ListEntries(ws.ID, feed.ID, 0)
		feed.EntryCount = len(entries)
		_ = h.store.UpdateFeed(feed)
		h.store.AddAuditLog(model.AuditLog{
			ID:          model.GenerateID(),
			WorkspaceID: ws.ID,
			Action:      "feed.refresh",
			Detail:      "handle=" + handle + " new_entries=" + strconv.Itoa(newCount),
			At:          time.Now(),
		})
		writeRecord(w, r, feedFields(feed))
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeRecord(w, r, feedFields(feed))
	case http.MethodDelete:
		_ = h.store.DeleteFeed(feed.ID)
		h.store.AddAuditLog(model.AuditLog{
			ID:          model.GenerateID(),
			WorkspaceID: ws.ID,
			Action:      "feed.delete",
			Detail:      "handle=" + handle,
			At:          time.Now(),
		})
		writeOK(w, r, "feed deleted: "+handle)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET or DELETE")
	}
}

func (h *Handler) handleEntries(w http.ResponseWriter, r *http.Request) {
	ws := getWorkspace(r)
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET")
		return
	}
	feedHandle := r.URL.Query().Get("feed")
	search := r.URL.Query().Get("q")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	var entries []*model.Entry
	if search != "" {
		entries = h.store.SearchEntries(ws.ID, search, limit)
	} else {
		feedID := ""
		if feedHandle != "" {
			f, err := h.store.GetFeed(feedHandle, ws.ID)
			if err == nil {
				feedID = f.ID
			}
		}
		entries = h.store.ListEntries(ws.ID, feedID, limit)
	}
	var records [][]string
	for _, e := range entries {
		records = append(records, entryFields(e))
	}
	if len(records) == 0 {
		writeOK(w, r, "no entries found")
		return
	}
	writeRecords(w, r, records)
}

func (h *Handler) handleEntryDetail(w http.ResponseWriter, r *http.Request) {
	ws := getWorkspace(r)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/entries/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, r, http.StatusBadRequest, "missing entry handle", "provide entry handle in URL: /entries/entry_xxxxx")
		return
	}
	handle := parts[0]
	entry, err := h.store.GetEntry(handle, ws.ID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "entry not found", "check the handle with GET /entries")
		return
	}
	if len(parts) >= 2 {
		action := parts[1]
		switch action {
		case "read":
			if r.Method != http.MethodPost {
				writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
				return
			}
			entry.Read = true
			_ = h.store.UpdateEntry(entry)
			writeOK(w, r, "entry marked as read: "+handle)
		case "unread":
			if r.Method != http.MethodPost {
				writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
				return
			}
			entry.Read = false
			_ = h.store.UpdateEntry(entry)
			writeOK(w, r, "entry marked as unread: "+handle)
		case "star":
			if r.Method != http.MethodPost {
				writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
				return
			}
			entry.Starred = true
			_ = h.store.UpdateEntry(entry)
			writeOK(w, r, "entry starred: "+handle)
		case "unstar":
			if r.Method != http.MethodPost {
				writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
				return
			}
			entry.Starred = false
			_ = h.store.UpdateEntry(entry)
			writeOK(w, r, "entry unstarred: "+handle)
		default:
			writeError(w, r, http.StatusBadRequest, "unknown action: "+action, "use /read, /unread, /star, or /unstar")
		}
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET")
		return
	}
	writeRecord(w, r, entryFields(entry))
}

func (h *Handler) handleAudit(w http.ResponseWriter, r *http.Request) {
	ws := getWorkspace(r)
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET")
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	logs := h.store.ListAuditLogs(ws.ID, limit)
	var records [][]string
	for _, a := range logs {
		records = append(records, []string{
			"action=" + a.Action,
			"detail=" + a.Detail,
			"at=" + a.At.Format(time.RFC3339),
		})
	}
	if len(records) == 0 {
		writeOK(w, r, "no audit logs found")
		return
	}
	writeRecords(w, r, records)
}

func feedFields(f *model.Feed) []string {
	lastRefresh := "never"
	if f.LastRefreshed != nil {
		lastRefresh = f.LastRefreshed.Format(time.RFC3339)
	}
	return []string{
		"handle=" + f.Handle,
		"url=" + f.URL,
		"title=" + f.Title,
		"entries=" + strconv.Itoa(f.EntryCount),
		"last_refreshed=" + lastRefresh,
	}
}

func entryFields(e *model.Entry) []string {
	pub := "unknown"
	if e.PublishedAt != nil {
		pub = e.PublishedAt.Format(time.RFC3339)
	}
	read := "no"
	if e.Read {
		read = "yes"
	}
	starred := "no"
	if e.Starred {
		starred = "yes"
	}
	return []string{
		"handle=" + e.Handle,
		"title=" + e.Title,
		"link=" + e.Link,
		"published=" + pub,
		"read=" + read,
		"starred=" + starred,
	}
}

const helpText = `feedkit — Agentic-first RSS/Atom feed reader and aggregator

The agent IS the interface. No UI, no SDK. Plain text by default.
JSON available via Accept: application/json or ?format=json.

AUTH:
  1. POST /auth/request  email=user@example.com
     → ok: OTP sent (code shown in dev mode)
  2. POST /auth/verify   email=user@example.com code=123456
     → token=xxx workspace=ws_xxx email=user@example.com
  3. Use: Authorization: Bearer <token>

FEEDS:
  POST /feeds            url=https://example.com/feed.xml
     → handle=feed_xxx url=... title=... entries=10 last_refreshed=...
  GET  /feeds            → list all feeds
  GET  /feeds/{handle}   → feed details
  POST /feeds/{handle}/refresh → fetch new entries from source
  DELETE /feeds/{handle} → remove feed and its entries

ENTRIES:
  GET /entries?feed=feed_xxx&limit=20 → list entries (newest first)
  GET /entries?q=keyword              → search entries by title/summary
  GET /entries/{handle}               → entry details
  POST /entries/{handle}/read         → mark as read
  POST /entries/{handle}/unread       → mark as unread
  POST /entries/{handle}/star         → star entry
  POST /entries/{handle}/unstar       → unstar entry

WORKSPACE:
  GET  /workspaces      → current workspace info
  POST /workspaces name=New Name → update workspace name

AUDIT:
  GET /audit?limit=50   → recent audit log entries

OTHER:
  GET /help             → this help text
  GET /.well-known/agent.md → same as /help
  GET /health           → health check
  POST /mcp             → MCP JSON-RPC 2.0 endpoint

ERRORS:
  Every 4xx includes: error: <message> | hint: <what to do next>

CONFIG:
  FEEDKIT_ADDR  listen address (default :8790)
  FEEDKIT_DB    database file (default feedkit.json)
  FEEDKIT_SECRET  token signing secret (auto-generated if not set)

FLAGS:
  -addr   listen address
  -db     database file path
  -secret token signing secret
`
