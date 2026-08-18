# feedkit

Agentic-first RSS/Atom feed reader and aggregator. Subscribe to feeds, fetch entries, search across all subscriptions. Plain text API, agent-driven, single Go binary with JSON file storage.

## Quick Start

```bash
# Build
make build

# Run (defaults to :8790, stores data in feedkit.json)
./feedkit

# Or with custom config
./feedkit -addr :3000 -db /var/lib/feedkit/data.json
```

## How It Works

feedkit is designed for AI agents, not humans. No UI, no SDK. The API is the product.

1. **Authenticate**: Request an OTP via email, verify it to get a bearer token.
2. **Subscribe to feeds**: Add RSS/Atom feed URLs.
3. **Read entries**: List, search, and manage feed entries.
4. **Refresh feeds**: Fetch new entries from source URLs on demand.

## API Reference

### Auth

```
POST /auth/request  email=user@example.com
→ ok: OTP sent (code shown in dev mode)

POST /auth/verify   email=user@example.com code=123456
→ token=xxx workspace=ws_xxx email=user@example.com
```

Use the token as: `Authorization: Bearer <token>`

### Feeds

```
POST /feeds            url=https://example.com/feed.xml
→ handle=feed_xxx url=... title=... entries=10 last_refreshed=...

GET  /feeds
→ handle=feed_xxx url=... title=... entries=10 ...
→ handle=feed_yyy url=... title=... entries=5 ...

GET  /feeds/{handle}
→ handle=feed_xxx url=... title=... entries=10 last_refreshed=...

POST /feeds/{handle}/refresh
→ handle=feed_xxx url=... title=... entries=12 last_refreshed=...

DELETE /feeds/{handle}
→ ok: feed deleted: feed_xxx
```

### Entries

```
GET /entries?feed=feed_xxx&limit=20
→ handle=entry_xxx title=... link=... published=... read=no starred=no
→ handle=entry_yyy title=... link=... published=... read=no starred=no

GET /entries?q=keyword
→ (search results matching title or summary)

GET /entries/{handle}
→ handle=entry_xxx title=... link=... published=... read=no starred=no

POST /entries/{handle}/read
→ ok: entry marked as read: entry_xxx

POST /entries/{handle}/unread
→ ok: entry marked as unread: entry_xxx

POST /entries/{handle}/star
→ ok: entry starred: entry_xxx

POST /entries/{handle}/unstar
→ ok: entry unstarred: entry_xxx
```

### Workspace

```
GET  /workspaces
→ handle=ws_xxx name=... email=... plan=free created=...

POST /workspaces name=New Name
→ handle=ws_xxx name=New Name email=... plan=free
```

### Audit

```
GET /audit?limit=50
→ action=feed.create detail=url=... at=2026-...
→ action=feed.refresh detail=handle=... new_entries=3 at=2026-...
```

### Other

```
GET /help              → operating manual (plain text)
GET /.well-known/agent.md → same as /help
GET /health            → ok: healthy
POST /mcp              → MCP JSON-RPC 2.0 endpoint
```

### Response Formats

- **Plain text** (default): One labeled, grepable line per record.
- **JSON**: Add `Accept: application/json` header or `?format=json` query param.

### Errors

Every 4xx response includes a hint:

```
error: missing auth token | hint: call POST /auth/request with email to get an OTP, then POST /auth/verify to get a bearer token
```

## Configuration

| Source | Variable | Default | Description |
|--------|----------|---------|-------------|
| Env | `FEEDKIT_ADDR` | `:8790` | Listen address |
| Env | `FEEDKIT_DB` | `feedkit.json` | Database file path |
| Env | `FEEDKIT_SECRET` | (random) | Token signing secret |
| Flag | `-addr` | `:8790` | Listen address |
| Flag | `-db` | `feedkit.json` | Database file path |
| Flag | `-secret` | (random) | Token signing secret |

Config priority: defaults < env vars < flags.

## MCP

feedkit speaks Model Context Protocol at `/mcp` for chat client integrations. Available tools:

- `list_feeds` — List all feed subscriptions
- `add_feed` — Subscribe to a new feed (url)
- `get_feed` — Get feed details (handle)
- `refresh_feed` — Refresh a feed (handle)
- `delete_feed` — Delete a feed (handle)
- `list_entries` — List entries (feed, limit, q)
- `get_entry` — Get entry details (handle)
- `mark_read` — Mark entry as read (handle)
- `star_entry` — Star an entry (handle)

## Build

```bash
make build    # CGO_ENABLED=0 go build -trimpath
make test     # go test ./...
make vet      # go vet ./...
make run      # go run ./cmd/feedkit
make clean    # remove binary and data files
```

## Tech Stack

- **Language**: Go
- **Storage**: JSON file (no external database)
- **Dependencies**: Zero external runtime dependencies
- **Binary**: Single static binary, CGO_ENABLED=0

## License

MIT
