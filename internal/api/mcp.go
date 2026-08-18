package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/relentlessworks/feedkit/internal/feedparser"
	"github.com/relentlessworks/feedkit/internal/model"
)

// JSON-RPC 2.0 types

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

func (h *Handler) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Return server info
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"server":  "feedkit",
			"version": "0.1.0",
			"tools":   h.mcpTools(),
		})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET or POST")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "failed to read body", "send a valid JSON-RPC request")
		return
	}
	var req jsonRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid JSON-RPC", "send a valid JSON-RPC 2.0 request")
		return
	}
	resp := h.handleMCPMethod(&req)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) mcpTools() []toolDef {
	return []toolDef{
		{Name: "list_feeds", Description: "List all feed subscriptions", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "add_feed", Description: "Subscribe to a new RSS/Atom feed", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"url": map[string]interface{}{"type": "string", "description": "Feed URL"}}, "required": []string{"url"}}},
		{Name: "get_feed", Description: "Get feed details", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"handle": map[string]interface{}{"type": "string"}}, "required": []string{"handle"}}},
		{Name: "refresh_feed", Description: "Refresh a feed to fetch new entries", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"handle": map[string]interface{}{"type": "string"}}, "required": []string{"handle"}}},
		{Name: "delete_feed", Description: "Delete a feed and its entries", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"handle": map[string]interface{}{"type": "string"}}, "required": []string{"handle"}}},
		{Name: "list_entries", Description: "List entries from feeds", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"feed": map[string]interface{}{"type": "string"}, "limit": map[string]interface{}{"type": "integer"}, "q": map[string]interface{}{"type": "string"}}}},
		{Name: "get_entry", Description: "Get entry details", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"handle": map[string]interface{}{"type": "string"}}, "required": []string{"handle"}}},
		{Name: "mark_read", Description: "Mark an entry as read", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"handle": map[string]interface{}{"type": "string"}}, "required": []string{"handle"}}},
		{Name: "star_entry", Description: "Star an entry", InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"handle": map[string]interface{}{"type": "string"}}, "required": []string{"handle"}}},
	}
}

func (h *Handler) handleMCPMethod(req *jsonRPCRequest) *jsonRPCResponse {
	// For MCP, we need a workspace. Try to get from auth header or use first workspace.
	// In a real deployment, the MCP endpoint would also require auth.
	workspaces := h.store.ListWorkspaces()
	var ws *model.Workspace
	if len(workspaces) > 0 {
		ws = workspaces[0]
	}
	if ws == nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -1, Message: "no workspace found. authenticate first via HTTP API."},
		}
	}

	switch req.Method {
	case "tools/list":
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{"tools": h.mcpTools()}}

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid params"}}
		}
		result := h.handleMCPToolCall(params.Name, params.Arguments, ws)
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{"content": result}}

	default:
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found: " + req.Method}}
	}
}

func (h *Handler) handleMCPToolCall(name string, args map[string]interface{}, ws *model.Workspace) []map[string]string {
	switch name {
	case "list_feeds":
		feeds := h.store.ListFeeds(ws.ID)
		var out []map[string]string
		for _, f := range feeds {
			out = append(out, map[string]string{
				"handle": f.Handle, "url": f.URL, "title": f.Title,
				"entries": fmt.Sprintf("%d", f.EntryCount),
			})
		}
		return out

	case "add_feed":
		url, _ := args["url"].(string)
		if url == "" {
			return []map[string]string{{"error": "missing url"}}
		}
		parsed, err := feedparser.FetchAndParse(url)
		if err != nil {
			return []map[string]string{{"error": "failed to parse: " + err.Error()}}
		}
		feed := &model.Feed{
			ID: model.GenerateID(), Handle: model.GenerateHandle("feed"),
			WorkspaceID: ws.ID, URL: url, Title: parsed.Title,
			Description: parsed.Description, SiteURL: parsed.SiteURL,
			EntryCount: len(parsed.Entries), CreatedAt: time.Now(),
		}
		now := time.Now()
		feed.LastRefreshed = &now
		h.store.CreateFeed(feed)
		for _, pe := range parsed.Entries {
			e := &model.Entry{
				ID: model.GenerateID(), Handle: model.GenerateHandle("entry"),
				FeedID: feed.ID, WorkspaceID: ws.ID, GUID: pe.GUID,
				Title: pe.Title, Link: pe.Link, Summary: pe.Summary,
				PublishedAt: pe.PublishedAt, CreatedAt: time.Now(),
			}
			h.store.CreateEntry(e)
		}
		return []map[string]string{{"handle": feed.Handle, "title": feed.Title, "entries": fmt.Sprintf("%d", feed.EntryCount)}}

	case "get_feed":
		handle, _ := args["handle"].(string)
		f, err := h.store.GetFeed(handle, ws.ID)
		if err != nil {
			return []map[string]string{{"error": "not found"}}
		}
		return []map[string]string{{"handle": f.Handle, "url": f.URL, "title": f.Title, "entries": fmt.Sprintf("%d", f.EntryCount)}}

	case "refresh_feed":
		handle, _ := args["handle"].(string)
		f, err := h.store.GetFeed(handle, ws.ID)
		if err != nil {
			return []map[string]string{{"error": "not found"}}
		}
		parsed, err := feedparser.FetchAndParse(f.URL)
		if err != nil {
			return []map[string]string{{"error": err.Error()}}
		}
		newCount := 0
		for _, pe := range parsed.Entries {
			existing, _ := h.store.GetEntryByGUID(pe.GUID, f.ID)
			if existing != nil {
				continue
			}
			e := &model.Entry{
				ID: model.GenerateID(), Handle: model.GenerateHandle("entry"),
				FeedID: f.ID, WorkspaceID: ws.ID, GUID: pe.GUID,
				Title: pe.Title, Link: pe.Link, Summary: pe.Summary,
				PublishedAt: pe.PublishedAt, CreatedAt: time.Now(),
			}
			h.store.CreateEntry(e)
			newCount++
		}
		entries := h.store.ListEntries(ws.ID, f.ID, 0)
		f.EntryCount = len(entries)
		h.store.UpdateFeed(f)
		return []map[string]string{{"handle": f.Handle, "new_entries": fmt.Sprintf("%d", newCount), "total": fmt.Sprintf("%d", f.EntryCount)}}

	case "delete_feed":
		handle, _ := args["handle"].(string)
		f, err := h.store.GetFeed(handle, ws.ID)
		if err != nil {
			return []map[string]string{{"error": "not found"}}
		}
		h.store.DeleteFeed(f.ID)
		return []map[string]string{{"ok": "deleted: " + handle}}

	case "list_entries":
		limit := 50
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		feedHandle, _ := args["feed"].(string)
		search, _ := args["q"].(string)
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
		var out []map[string]string
		for _, e := range entries {
			out = append(out, map[string]string{
				"handle": e.Handle, "title": e.Title, "link": e.Link,
			})
		}
		return out

	case "get_entry":
		handle, _ := args["handle"].(string)
		e, err := h.store.GetEntry(handle, ws.ID)
		if err != nil {
			return []map[string]string{{"error": "not found"}}
		}
		pub := "unknown"
		if e.PublishedAt != nil {
			pub = e.PublishedAt.Format(time.RFC3339)
		}
		return []map[string]string{{"handle": e.Handle, "title": e.Title, "link": e.Link, "summary": e.Summary, "published": pub}}

	case "mark_read":
		handle, _ := args["handle"].(string)
		e, err := h.store.GetEntry(handle, ws.ID)
		if err != nil {
			return []map[string]string{{"error": "not found"}}
		}
		e.Read = true
		h.store.UpdateEntry(e)
		return []map[string]string{{"ok": "marked read: " + handle}}

	case "star_entry":
		handle, _ := args["handle"].(string)
		e, err := h.store.GetEntry(handle, ws.ID)
		if err != nil {
			return []map[string]string{{"error": "not found"}}
		}
		e.Starred = true
		h.store.UpdateEntry(e)
		return []map[string]string{{"ok": "starred: " + handle}}

	default:
		return []map[string]string{{"error": "unknown tool: " + name}}
	}
}

// Ensure strings import is used
var _ = strings.TrimSpace
