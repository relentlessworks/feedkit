package api

import (
	"encoding/json"
	"io"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/relentlessworks/feedkit/internal/auth"
	"github.com/relentlessworks/feedkit/internal/feedparser"
	"github.com/relentlessworks/feedkit/internal/model"
	"github.com/relentlessworks/feedkit/internal/store"
)

const testRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <description>A test RSS feed</description>
    <link>https://example.com</link>
    <item>
      <title>First Post</title>
      <link>https://example.com/post1</link>
      <description>This is the first post</description>
      <guid>https://example.com/post1</guid>
      <pubDate>Mon, 18 Aug 2026 10:00:00 GMT</pubDate>
    </item>
    <item>
      <title>Second Post</title>
      <link>https://example.com/post2</link>
      <description>This is the second post</description>
      <guid>https://example.com/post2</guid>
      <pubDate>Mon, 17 Aug 2026 10:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

const testAtom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Test Feed</title>
  <subtitle>An atom test</subtitle>
  <link href="https://atom.example.com" rel="alternate"/>
  <entry>
    <title>Atom Entry 1</title>
    <link href="https://atom.example.com/1" rel="alternate"/>
    <id>atom-1</id>
    <updated>2026-08-18T10:00:00Z</updated>
    <summary>Atom summary 1</summary>
  </entry>
</feed>`

func testServer(t *testing.T) (*httptest.Server, *Handler) {
	t.Helper()
	s, err := store.New(fmt.Sprintf("/tmp/feedkit-test-%d.json", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	a := auth.New(s)
	h := New(s, a)
	ts := httptest.NewServer(h.Routes())
	return ts, h
}

func getToken(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	// Request OTP
	resp := postForm(ts.URL+"/auth/request", "email=test@example.com")
	if resp.StatusCode != 200 {
		t.Fatalf("auth request failed: %s", resp.Body)
	}
	// Extract code from response (dev mode shows it)
	body := resp.Body
	idx := strings.Index(body, "code: ")
	if idx < 0 {
		t.Fatalf("no code in response: %s", body)
	}
	rest := body[idx+6:]
	code := ""
	for _, c := range rest {
		if c >= '0' && c <= '9' {
			code += string(c)
		} else {
			break
		}
	}
	if len(code) != 6 {
		t.Fatalf("expected 6-digit code, got '%s' from: %s", code, body)
	}
	// Verify OTP
	resp = postForm(ts.URL+"/auth/verify", "email=test@example.com&code="+code)
	if resp.StatusCode != 200 {
		t.Fatalf("auth verify failed: %s", resp.Body)
	}
	// Extract token
	idx = strings.Index(resp.Body, "token=")
	if idx < 0 {
		t.Fatalf("no token in response: %s", resp.Body)
	}
	token := strings.TrimSpace(resp.Body[idx+6:])
	parts := strings.Fields(token)
	if len(parts) > 0 {
		token = parts[0]
	}
	return token
}

type resp struct {
	StatusCode int
	Body       string
}

func doGet(url string) resp {
	r, err := http.Get(url)
	if err != nil {
		return resp{StatusCode: 0, Body: err.Error()}
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return resp{StatusCode: r.StatusCode, Body: string(b)}
}

func doGetAuth(url, token string) resp {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return resp{StatusCode: 0, Body: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return resp{StatusCode: 0, Body: err.Error()}
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return resp{StatusCode: r.StatusCode, Body: string(b)}
}

func postForm(url, data string) resp {
	r, err := http.Post(url, "application/x-www-form-urlencoded", strings.NewReader(data))
	if err != nil {
		return resp{StatusCode: 0, Body: err.Error()}
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return resp{StatusCode: r.StatusCode, Body: string(b)}
}

func postFormAuth(url, data, token string) resp {
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return resp{StatusCode: 0, Body: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return resp{StatusCode: 0, Body: err.Error()}
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return resp{StatusCode: r.StatusCode, Body: string(b)}
}

func deleteAuth(url, token string) resp {
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return resp{StatusCode: 0, Body: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return resp{StatusCode: 0, Body: err.Error()}
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return resp{StatusCode: r.StatusCode, Body: string(b)}
}

func TestAuthFlow(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()
	token := getToken(t, ts)
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestHelp(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()
	r := doGet(ts.URL + "/help")
	if r.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", r.StatusCode, r.Body)
	}
	if !strings.Contains(r.Body, "feedkit") {
		t.Fatalf("help should mention feedkit: %s", r.Body)
	}
}

func TestHealth(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()
	r := doGet(ts.URL + "/health")
	if r.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", r.StatusCode)
	}
}

func TestFeedParserRSS(t *testing.T) {
	pf, err := feedparser.Parse([]byte(testRSS))
	if err != nil {
		t.Fatal(err)
	}
	if pf.Title != "Test Feed" {
		t.Fatalf("expected 'Test Feed', got '%s'", pf.Title)
	}
	if len(pf.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(pf.Entries))
	}
	if pf.Entries[0].Title != "First Post" {
		t.Fatalf("expected 'First Post', got '%s'", pf.Entries[0].Title)
	}
}

func TestFeedParserAtom(t *testing.T) {
	pf, err := feedparser.Parse([]byte(testAtom))
	if err != nil {
		t.Fatal(err)
	}
	if pf.Title != "Atom Test Feed" {
		t.Fatalf("expected 'Atom Test Feed', got '%s'", pf.Title)
	}
	if len(pf.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(pf.Entries))
	}
	if pf.Entries[0].Title != "Atom Entry 1" {
		t.Fatalf("expected 'Atom Entry 1', got '%s'", pf.Entries[0].Title)
	}
}

func TestFeedCRUD(t *testing.T) {
	// Start a test RSS server
	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, testRSS)
	}))
	defer rssServer.Close()

	ts, _ := testServer(t)
	defer ts.Close()
	token := getToken(t, ts)

	// Create feed
	r := postFormAuth(ts.URL+"/feeds", "url="+rssServer.URL, token)
	if r.StatusCode != 200 {
		t.Fatalf("create feed failed: %d: %s", r.StatusCode, r.Body)
	}
	if !strings.Contains(r.Body, "handle=feed_") {
		t.Fatalf("expected feed handle: %s", r.Body)
	}
	// Extract handle
	handle := extractField(r.Body, "handle=")

	// List feeds
	r = doGetAuth(ts.URL+"/feeds", token)
	if r.StatusCode != 200 {
		t.Fatalf("list feeds failed: %d", r.StatusCode)
	}
	if !strings.Contains(r.Body, handle) {
		t.Fatalf("expected feed in list: %s", r.Body)
	}

	// Get feed detail
	r = doGetAuth(ts.URL+"/feeds/"+handle, token)
	if r.StatusCode != 200 {
		t.Fatalf("get feed failed: %d", r.StatusCode)
	}
	if !strings.Contains(r.Body, "Test Feed") {
		t.Fatalf("expected feed title: %s", r.Body)
	}

	// List entries
	r = doGetAuth(ts.URL+"/entries", token)
	if r.StatusCode != 200 {
		t.Fatalf("list entries failed: %d: %s", r.StatusCode, r.Body)
	}
	if !strings.Contains(r.Body, "First Post") {
		t.Fatalf("expected entry: %s", r.Body)
	}

	// Search entries
	r = doGetAuth(ts.URL+"/entries?q=second", token)
	if r.StatusCode != 200 {
		t.Fatalf("search entries failed: %d", r.StatusCode)
	}
	if !strings.Contains(r.Body, "Second Post") {
		t.Fatalf("expected search result: %s", r.Body)
	}

	// Refresh feed
	r = postFormAuth(ts.URL+"/feeds/"+handle+"/refresh", "", token)
	if r.StatusCode != 200 {
		t.Fatalf("refresh feed failed: %d: %s", r.StatusCode, r.Body)
	}

	// Delete feed
	r = deleteAuth(ts.URL+"/feeds/"+handle, token)
	if r.StatusCode != 200 {
		t.Fatalf("delete feed failed: %d", r.StatusCode)
	}

	// Verify feed is gone
	r = doGetAuth(ts.URL+"/feeds/"+handle, token)
	if r.StatusCode == 200 {
		t.Fatal("expected feed to be deleted")
	}
}

func TestEntryActions(t *testing.T) {
	// Start a test RSS server
	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, testRSS)
	}))
	defer rssServer.Close()

	ts, _ := testServer(t)
	defer ts.Close()
	token := getToken(t, ts)

	// Create feed
	r := postFormAuth(ts.URL+"/feeds", "url="+rssServer.URL, token)
	if r.StatusCode != 200 {
		t.Fatalf("create feed failed: %d", r.StatusCode)
	}

	// List entries
	r = doGetAuth(ts.URL+"/entries", token)
	if r.StatusCode != 200 {
		t.Fatalf("list entries failed: %d", r.StatusCode)
	}
	entryHandle := extractField(r.Body, "handle=")
	if entryHandle == "" || !strings.HasPrefix(entryHandle, "entry_") {
		t.Fatalf("expected entry handle, got '%s': %s", entryHandle, r.Body)
	}

	// Mark as read
	r = postFormAuth(ts.URL+"/entries/"+entryHandle+"/read", "", token)
	if r.StatusCode != 200 {
		t.Fatalf("mark read failed: %d: %s", r.StatusCode, r.Body)
	}

	// Star entry
	r = postFormAuth(ts.URL+"/entries/"+entryHandle+"/star", "", token)
	if r.StatusCode != 200 {
		t.Fatalf("star failed: %d: %s", r.StatusCode, r.Body)
	}

	// Get entry detail
	r = doGetAuth(ts.URL+"/entries/"+entryHandle, token)
	if r.StatusCode != 200 {
		t.Fatalf("get entry failed: %d", r.StatusCode)
	}
	if !strings.Contains(r.Body, "read=yes") {
		t.Fatalf("expected read=yes: %s", r.Body)
	}
	if !strings.Contains(r.Body, "starred=yes") {
		t.Fatalf("expected starred=yes: %s", r.Body)
	}

	// Unstar
	r = postFormAuth(ts.URL+"/entries/"+entryHandle+"/unstar", "", token)
	if r.StatusCode != 200 {
		t.Fatalf("unstar failed: %d", r.StatusCode)
	}

	// Mark unread
	r = postFormAuth(ts.URL+"/entries/"+entryHandle+"/unread", "", token)
	if r.StatusCode != 200 {
		t.Fatalf("unread failed: %d", r.StatusCode)
	}
}

func TestJSONFormat(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()
	token := getToken(t, ts)

	// Get workspace as JSON
	req, err := http.NewRequest("GET", ts.URL+"/workspaces", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var m map[string]string
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		t.Fatalf("expected JSON, got error: %v", err)
	}
	if m["email"] != "test@example.com" {
		t.Fatalf("expected email, got %v", m)
	}
}

func TestAuditLog(t *testing.T) {
	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, testRSS)
	}))
	defer rssServer.Close()

	ts, _ := testServer(t)
	defer ts.Close()
	token := getToken(t, ts)

	// Create a feed to generate audit log
	_ = postFormAuth(ts.URL+"/feeds", "url="+rssServer.URL, token)

	// Get audit logs
	r := doGetAuth(ts.URL+"/audit", token)
	if r.StatusCode != 200 {
		t.Fatalf("get audit failed: %d", r.StatusCode)
	}
	if !strings.Contains(r.Body, "feed.create") {
		t.Fatalf("expected feed.create in audit: %s", r.Body)
	}
}

func TestMCP(t *testing.T) {
	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, testRSS)
	}))
	defer rssServer.Close()

	ts, _ := testServer(t)
	defer ts.Close()
	token := getToken(t, ts)

	// First, create a feed via HTTP API so workspace has data
	_ = postFormAuth(ts.URL+"/feeds", "url="+rssServer.URL, token)

	// MCP: list tools
	mcpReq := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	r, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(mcpReq))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var resp map[string]interface{}
	json.NewDecoder(r.Body).Decode(&resp)
	if resp["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0, got %v", resp)
	}

	// MCP: call list_feeds
	mcpReq = `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_feeds","arguments":{}}}`
	r2, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(mcpReq))
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	var resp2 map[string]interface{}
	json.NewDecoder(r2.Body).Decode(&resp2)
	if resp2["error"] != nil {
		t.Fatalf("list_feeds failed: %v", resp2)
	}
}

func TestNoAuth(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()
	// Try to access protected endpoint without auth
	r := doGet(ts.URL + "/feeds")
	if r.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", r.StatusCode)
	}
	if !strings.Contains(r.Body, "hint:") {
		t.Fatalf("expected hint in error: %s", r.Body)
	}
}

func TestDuplicateFeed(t *testing.T) {
	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, testRSS)
	}))
	defer rssServer.Close()

	ts, _ := testServer(t)
	defer ts.Close()
	token := getToken(t, ts)

	// Create feed
	r := postFormAuth(ts.URL+"/feeds", "url="+rssServer.URL, token)
	if r.StatusCode != 200 {
		t.Fatalf("first create failed: %d", r.StatusCode)
	}

	// Try to create same feed again
	r = postFormAuth(ts.URL+"/feeds", "url="+rssServer.URL, token)
	if r.StatusCode != 409 {
		t.Fatalf("expected 409 for duplicate, got %d: %s", r.StatusCode, r.Body)
	}
}

func TestWorkspaceUpdate(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()
	token := getToken(t, ts)

	// Update workspace name
	r := postFormAuth(ts.URL+"/workspaces", "name=MyWorkspace", token)
	if r.StatusCode != 200 {
		t.Fatalf("update workspace failed: %d: %s", r.StatusCode, r.Body)
	}
	if !strings.Contains(r.Body, "MyWorkspace") {
		t.Fatalf("expected new name: %s", r.Body)
	}
}

// Helper to extract a field value from plain text response
func extractField(body, prefix string) string {
	idx := strings.Index(body, prefix)
	if idx < 0 {
		return ""
	}
	rest := body[idx+len(prefix):]
	// Find end of value (space or newline)
	end := len(rest)
	for i, c := range rest {
		if c == ' ' || c == '\n' || c == '\r' {
			end = i
			break
		}
	}
	return rest[:end]
}

// Ensure model import is used
var _ = model.GenerateID
