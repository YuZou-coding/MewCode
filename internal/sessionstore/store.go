package sessionstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mewcode/internal/chat"
)

type Store struct {
	ProjectRoot string
	Now         func() time.Time
}

func (s Store) Create() (*SessionStore, error) {
	now := s.now()
	id := newID(now)
	dir := SessionDir(s.ProjectRoot, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	meta := Meta{ID: id, Title: "新会话 " + now.Format("2006-01-02 15:04"), Summary: "", UpdatedAt: now.Format(time.RFC3339)}
	ss := &SessionStore{ProjectRoot: s.ProjectRoot, ID: id, now: s.now, meta: meta}
	if err := ss.writeMeta(); err != nil {
		return nil, err
	}
	return ss, nil
}

func (s Store) Open(id string) *SessionStore {
	return &SessionStore{ProjectRoot: s.ProjectRoot, ID: id, now: s.now}
}

func (s Store) List() ([]Meta, error) {
	root := Root(s.ProjectRoot)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var metas []Meta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := readMeta(filepath.Join(root, entry.Name(), MetaFile))
		if err == nil {
			metas = append(metas, meta)
		}
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].UpdatedAt > metas[j].UpdatedAt
	})
	return metas, nil
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type SessionStore struct {
	ProjectRoot string
	ID          string
	now         func() time.Time
	meta        Meta
}

func (s *SessionStore) Append(message chat.Message) error {
	if s == nil {
		return nil
	}
	now := s.currentTime()
	dir := SessionDir(s.ProjectRoot, s.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	record := Record{
		Role:       message.Role,
		Content:    message.Content,
		ToolCall:   message.ToolCall,
		ToolCalls:  message.ToolCalls,
		ToolResult: message.ToolResult,
		CreatedAt:  now.Format(time.RFC3339),
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, MessagesFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(raw, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return s.updateMeta(message, now)
}

func (s *SessionStore) Meta() (Meta, error) {
	if s.meta.ID != "" {
		return s.meta, nil
	}
	meta, err := readMeta(filepath.Join(SessionDir(s.ProjectRoot, s.ID), MetaFile))
	if err != nil {
		return Meta{}, err
	}
	s.meta = meta
	return meta, nil
}

func (s *SessionStore) writeMeta() error {
	dir := SessionDir(s.ProjectRoot, s.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, MetaFile), raw, 0600)
}

func (s *SessionStore) updateMeta(message chat.Message, now time.Time) error {
	meta, err := s.Meta()
	if err != nil {
		meta = Meta{ID: s.ID}
	}
	meta.MessageCount++
	meta.UpdatedAt = now.Format(time.RFC3339)
	if strings.TrimSpace(meta.Title) == "" || strings.HasPrefix(meta.Title, "新会话 ") {
		if message.Role == chat.RoleUser && strings.TrimSpace(message.Content) != "" {
			meta.Title = shortTitle(message.Content)
		}
	}
	s.meta = meta
	return s.writeMeta()
}

func (s *SessionStore) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func readMeta(path string) (Meta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	var meta Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

func shortTitle(content string) string {
	content = strings.TrimSpace(strings.Join(strings.Fields(content), " "))
	runes := []rune(content)
	if len(runes) > 32 {
		return string(runes[:32])
	}
	return content
}

func newID(now time.Time) string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return now.Format("20060102-150405") + "-000000"
	}
	return fmt.Sprintf("%s-%s", now.Format("20060102-150405"), hex.EncodeToString(b[:]))
}
