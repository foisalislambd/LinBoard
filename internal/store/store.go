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

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func makePreview(content string, contentType ContentType) string {
	switch contentType {
	case TypeImage:
		return "[Image]"
	case TypeURL:
		return truncateRunes(content, 120)
	default:
		lines := strings.Split(strings.TrimSpace(content), "\n")
		first := truncateRunes(lines[0], 120)
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
		prev := s.clips[i].CreatedAt
		s.clips[i].CreatedAt = now
		if err := s.saveLocked(); err != nil {
			s.clips[i].CreatedAt = prev
			return nil, err
		}
		c := s.clips[i]
		return &c, nil
	}

	id := s.nextID
	snapshot := cloneClips(s.clips)
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
	dropped := s.pruneInMemory()
	if err := s.saveLocked(); err != nil {
		s.clips = snapshot
		s.nextID = id
		return nil, err
	}
	removeClipFiles(dropped)
	if kept := s.findByID(id); kept != nil {
		cp := *kept
		return &cp, nil
	}
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
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, err
		}
	}

	kb := (len(data) + 1023) / 1024
	preview := fmt.Sprintf("[Image · %d KB]", kb)
	if i, existing := s.findByHash(hash); existing != nil {
		prev := s.clips[i].CreatedAt
		s.clips[i].CreatedAt = now
		if err := s.saveLocked(); err != nil {
			s.clips[i].CreatedAt = prev
			return nil, err
		}
		c := s.clips[i]
		return &c, nil
	}

	id := s.nextID
	snapshot := cloneClips(s.clips)
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
	dropped := s.pruneInMemory()
	if err := s.saveLocked(); err != nil {
		s.clips = snapshot
		s.nextID = id
		return nil, err
	}
	removeClipFiles(dropped)
	if kept := s.findByID(id); kept != nil {
		cp := *kept
		return &cp, nil
	}
	return &c, nil
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
		dropped := s.clips[i]
		s.clips = append(s.clips[:i], s.clips[i+1:]...)
		if err := s.saveLocked(); err != nil {
			s.clips = append(s.clips[:i], append([]Clip{dropped}, s.clips[i:]...)...)
			return err
		}
		removeClipFiles([]Clip{dropped})
		return nil
	}
	return nil
}

func (s *Store) ClearUnpinned() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := cloneClips(s.clips)
	out := make([]Clip, 0, len(s.clips))
	var dropped []Clip
	for _, c := range s.clips {
		if c.Pinned {
			out = append(out, c)
			continue
		}
		dropped = append(dropped, c)
	}
	s.clips = out
	if err := s.saveLocked(); err != nil {
		s.clips = snapshot
		return err
	}
	removeClipFiles(dropped)
	return nil
}

func (s *Store) Count() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clips), nil
}

func cloneClips(in []Clip) []Clip {
	out := make([]Clip, len(in))
	copy(out, in)
	return out
}

func FormatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)
	if diff < 0 {
		return "Just now"
	}
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
