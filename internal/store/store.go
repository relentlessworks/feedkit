package store

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/relentlessworks/feedkit/internal/model"
)

// Store is a JSON file-backed data store.
type Store struct {
	mu   sync.RWMutex
	path string
	data *Data
}

// Data is the on-disk JSON structure.
type Data struct {
	Workspaces map[string]*model.Workspace `json:"workspaces"`
	Feeds      map[string]*model.Feed      `json:"feeds"`
	Entries    map[string]*model.Entry     `json:"entries"`
	Tokens     map[string]*model.Token     `json:"tokens"`
	OTPs       map[string]*model.OTP       `json:"otps"`
	AuditLogs  []model.AuditLog            `json:"audit_logs"`
}

// New creates a new Store backed by the given file path.
func New(path string) (*Store, error) {
	s := &Store{
		path: path,
		data: &Data{
			Workspaces: make(map[string]*model.Workspace),
			Feeds:      make(map[string]*model.Feed),
			Entries:    make(map[string]*model.Entry),
			Tokens:     make(map[string]*model.Token),
			OTPs:       make(map[string]*model.OTP),
		},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, s.data)
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0644)
}

// --- Workspace ---

func (s *Store) CreateWorkspace(w *model.Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Workspaces[w.ID] = w
	return s.save()
}

func (s *Store) GetWorkspaceByHandle(handle string) (*model.Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, w := range s.data.Workspaces {
		if w.Handle == handle {
			return w, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) GetWorkspaceByEmail(email string) (*model.Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, w := range s.data.Workspaces {
		if w.Email == email {
			return w, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) ListWorkspaces() []*model.Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []*model.Workspace
	for _, w := range s.data.Workspaces {
		list = append(list, w)
	}
	return list
}

// --- Feed ---

func (s *Store) CreateFeed(f *model.Feed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Feeds[f.ID] = f
	return s.save()
}

func (s *Store) GetFeed(handle, wsID string) (*model.Feed, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.data.Feeds {
		if f.Handle == handle && f.WorkspaceID == wsID {
			return f, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) GetFeedByURL(url, wsID string) (*model.Feed, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.data.Feeds {
		if f.URL == url && f.WorkspaceID == wsID {
			return f, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) ListFeeds(wsID string) []*model.Feed {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []*model.Feed
	for _, f := range s.data.Feeds {
		if f.WorkspaceID == wsID {
			list = append(list, f)
		}
	}
	return list
}

func (s *Store) UpdateFeed(f *model.Feed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Feeds[f.ID] = f
	return s.save()
}

func (s *Store) DeleteFeed(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Feeds, id)
	for eid, e := range s.data.Entries {
		if e.FeedID == id {
			delete(s.data.Entries, eid)
		}
	}
	return s.save()
}

// --- Entry ---

func (s *Store) CreateEntry(e *model.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Entries[e.ID] = e
	return s.save()
}

func (s *Store) GetEntry(handle, wsID string) (*model.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.data.Entries {
		if e.Handle == handle && e.WorkspaceID == wsID {
			return e, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) GetEntryByGUID(guid, feedID string) (*model.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.data.Entries {
		if e.GUID == guid && e.FeedID == feedID {
			return e, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) ListEntries(wsID string, feedID string, limit int) []*model.Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []*model.Entry
	for _, e := range s.data.Entries {
		if e.WorkspaceID != wsID {
			continue
		}
		if feedID != "" && e.FeedID != feedID {
			continue
		}
		list = append(list, e)
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			var ti, tj time.Time
			if list[i].PublishedAt != nil {
				ti = *list[i].PublishedAt
			}
			if list[j].PublishedAt != nil {
				tj = *list[j].PublishedAt
			}
			if tj.After(ti) {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list
}

func (s *Store) SearchEntries(wsID, query string, limit int) []*model.Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(query)
	var list []*model.Entry
	for _, e := range s.data.Entries {
		if e.WorkspaceID != wsID {
			continue
		}
		if strings.Contains(strings.ToLower(e.Title), q) ||
			strings.Contains(strings.ToLower(e.Summary), q) {
			list = append(list, e)
		}
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list
}

func (s *Store) UpdateEntry(e *model.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Entries[e.ID] = e
	return s.save()
}

// --- Token ---

func (s *Store) CreateToken(t *model.Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Tokens[t.ID] = t
	return s.save()
}

func (s *Store) GetToken(token string) (*model.Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.data.Tokens {
		if t.Token == token {
			return t, nil
		}
	}
	return nil, ErrNotFound
}

// --- OTP ---

func (s *Store) SaveOTP(o *model.OTP) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.OTPs[o.Email] = o
	return s.save()
}

func (s *Store) GetOTP(email string) (*model.OTP, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o, ok := s.data.OTPs[email]
	if !ok {
		return nil, ErrNotFound
	}
	return o, nil
}

func (s *Store) DeleteOTP(email string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.OTPs, email)
	_ = s.save()
}

// --- Audit ---

func (s *Store) AddAuditLog(a model.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.AuditLogs = append(s.data.AuditLogs, a)
	return s.save()
}

func (s *Store) ListAuditLogs(wsID string, limit int) []model.AuditLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []model.AuditLog
	for _, a := range s.data.AuditLogs {
		if a.WorkspaceID == wsID || a.WorkspaceID == "" {
			list = append(list, a)
		}
	}
	if limit > 0 && len(list) > limit {
		list = list[len(list)-limit:]
	}
	return list
}
