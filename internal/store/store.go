package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/foisal/linboard/internal/config"
)

type ContentType string

const (
	TypeText  ContentType = "text"
	TypeImage ContentType = "image"
	TypeURL   ContentType = "url"
)

type Clip struct {
	ID          int64       `json:"id"`
	Content     string      `json:"content"`
	ContentType ContentType `json:"content_type"`
	ImagePath   string      `json:"image_path,omitempty"`
	Preview     string      `json:"preview"`
	Pinned      bool        `json:"pinned"`
	CreatedAt   time.Time   `json:"created_at"`
	Hash        string      `json:"hash"`
}

type Store struct {
	*fileStore
}

func Open(maxItems int) (*Store, error) {
	fs, err := openFileStore(maxItems)
	if err != nil {
		return nil, err
	}
	return &Store{fileStore: fs}, nil
}

func (s *Store) Close() error {
	return nil
}

func hashContent(content string, contentType ContentType) string {
	h := sha256.Sum256([]byte(string(contentType) + ":" + content))
	return hex.EncodeToString(h[:])
}

func hashBytes(data []byte, contentType ContentType) string {
	h := sha256.New()
	h.Write([]byte(string(contentType) + ":"))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func makePreview(content string, contentType ContentType) string {
	switch contentType {
	case TypeImage:
		return "[Image]"
	case TypeURL:
		if len(content) > 120 {
			return content[:120] + "…"
		}
		return content
	default:
		lines := strings.Split(strings.TrimSpace(content), "\n")
		first := lines[0]
		if len(first) > 120 {
			first = first[:120] + "…"
		}
		if len(lines) > 1 {
			first += fmt.Sprintf(" (+%d lines)", len(lines)-1)
		}
		return first
	}
}

func detectType(content string) ContentType {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		if !strings.Contains(trimmed, " ") && len(trimmed) < 2048 {
			return TypeURL
		}
	}
	return TypeText
}

func (s *Store) AddText(content string) (*Clip, error) {
	content = strings.TrimRight(content, "\x00")
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ct := detectType(content)
	hash := hashContent(content, ct)
	now := time.Now()

	if i, existing := s.findByHash(hash); existing != nil {
		s.clips[i].CreatedAt = now
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
		s.pruneLocked()
		c := s.clips[i]
		return &c, nil
	}

	id := s.nextID
	s.nextID++
	c := Clip{
		ID:          id,
		Content:     content,
		ContentType: ct,
		Preview:     makePreview(content, ct),
		CreatedAt:   now,
		Hash:        hash,
	}
	s.clips = append(s.clips, c)
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	s.pruneLocked()
	return &c, nil
}

func (s *Store) AddImage(data []byte) (*Clip, error) {
	if len(data) == 0 {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	hash := hashBytes(data, TypeImage)
	now := time.Now()

	filename := hash[:16] + ".png"
	path := filepath.Join(s.imagesDir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, err
		}
	}

	preview := fmt.Sprintf("[Image · %d KB]", len(data)/1024)
	if i, existing := s.findByHash(hash); existing != nil {
		s.clips[i].CreatedAt = now
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
		s.pruneLocked()
		c := s.clips[i]
		return &c, nil
	}

	id := s.nextID
	s.nextID++
	c := Clip{
		ID:          id,
		ContentType: TypeImage,
		ImagePath:   path,
		Preview:     preview,
		CreatedAt:   now,
		Hash:        hash,
	}
	s.clips = append(s.clips, c)
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	s.pruneLocked()
	return &c, nil
}

func (s *Store) pruneLocked() {
	if s.maxItems < 1 || len(s.clips) <= s.maxItems {
		return
	}
	indices := s.sortedIndices()
	keep := make(map[int64]bool, len(s.clips))
	pinnedN := 0
	for _, idx := range indices {
		c := s.clips[idx]
		if !c.Pinned {
			continue
		}
		keep[c.ID] = true
		pinnedN++
	}
	// Fill remaining capacity with newest unpinned clips.
	remain := s.maxItems - pinnedN
	if remain < 0 {
		remain = 0 // pinned alone may exceed max_history
	}
	for _, idx := range indices {
		if remain == 0 {
			break
		}
		c := s.clips[idx]
		if c.Pinned {
			continue
		}
		keep[c.ID] = true
		remain--
	}
	out := s.clips[:0]
	for i := range s.clips {
		c := s.clips[i]
		if keep[c.ID] {
			out = append(out, c)
			continue
		}
		if c.ImagePath != "" {
			_ = os.Remove(c.ImagePath)
		}
	}
	s.clips = out
	_ = s.saveLocked()
}

func (s *Store) List(search string, limit int) ([]Clip, error) {
	if limit <= 0 {
		limit = 50
	}
	search = strings.TrimSpace(strings.ToLower(search))

	s.mu.Lock()
	defer s.mu.Unlock()

	indices := s.sortedIndices()
	clips := make([]Clip, 0, limit)
	for _, i := range indices {
		c := s.clips[i]
		if search != "" {
			needle := strings.ToLower(c.Preview + " " + c.Content)
			if !strings.Contains(needle, search) {
				continue
			}
		}
		clips = append(clips, c)
		if len(clips) >= limit {
			break
		}
	}
	return clips, nil
}

func (s *Store) GetByID(id int64) (*Clip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.findByID(id); c != nil {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (s *Store) TogglePin(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.clips {
		if s.clips[i].ID == id {
			s.clips[i].Pinned = !s.clips[i].Pinned
			return s.saveLocked()
		}
	}
	return nil
}

func (s *Store) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.clips {
		if s.clips[i].ID != id {
			continue
		}
		if s.clips[i].ImagePath != "" {
			_ = os.Remove(s.clips[i].ImagePath)
		}
		s.clips = append(s.clips[:i], s.clips[i+1:]...)
		return s.saveLocked()
	}
	return nil
}

func (s *Store) ClearUnpinned() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.clips[:0]
	for _, c := range s.clips {
		if c.Pinned {
			out = append(out, c)
			continue
		}
		if c.ImagePath != "" {
			_ = os.Remove(c.ImagePath)
		}
	}
	s.clips = out
	return s.saveLocked()
}

func (s *Store) Count() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clips), nil
}

func FormatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)
	switch {
	case diff < time.Minute:
		return "Just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	default:
		return t.Format("Jan 2, 2006")
	}
}

// DataFilePath returns the native history file path (for diagnostics).
func DataFilePath() (string, error) {
	dir, err := config.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clips.json"), nil
}
