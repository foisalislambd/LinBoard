package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/fsutil"
)

const (
	// MaxTextBytes is the largest text clip stored. Bigger copies are skipped
	// so a single paste cannot stall history rewrites.
	MaxTextBytes = 1 << 20
	// MaxImageBytes is the largest image stored on disk.
	MaxImageBytes = 20 << 20
)

// ErrTooLarge is returned when a clipboard item exceeds MaxTextBytes or MaxImageBytes.
var ErrTooLarge = errors.New("clipboard item exceeds the storage limit")

// PNG header, used to reject non-image bytes from xclip.
var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// IsPNG reports whether data begins with a PNG signature.
func IsPNG(data []byte) bool {
	return len(data) >= len(pngMagic) && bytes.Equal(data[:len(pngMagic)], pngMagic)
}

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
	if len(content) > MaxTextBytes {
		return nil, ErrTooLarge
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
	s.removeClipFiles(dropped)
	if kept := s.findByID(id); kept != nil {
		cp := *kept
		return &cp, nil
	}
	return &c, nil
}

// IsLatestText reports whether content is already the newest stored clip.
// Callers use this at startup so an unchanged clipboard is not bumped to the top.
func (s *Store) IsLatestText(content string) bool {
	content = strings.TrimRight(content, "\x00")
	if strings.TrimSpace(content) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isLatestHash(hashContent(content, detectType(content)))
}

// IsLatestImage reports whether data is already the newest stored clip.
func (s *Store) IsLatestImage(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isLatestHash(hashBytes(data, TypeImage))
}

func (s *fileStore) isLatestHash(hash string) bool {
	var latest *Clip
	for i := range s.clips {
		c := &s.clips[i]
		if latest == nil || c.CreatedAt.After(latest.CreatedAt) || (c.CreatedAt.Equal(latest.CreatedAt) && c.ID > latest.ID) {
			latest = c
		}
	}
	return latest != nil && latest.Hash == hash
}

// ReadImage returns the PNG bytes for a clip, refusing paths outside the image directory.
func (s *Store) ReadImage(c *Clip) ([]byte, error) {
	if c == nil || c.ImagePath == "" {
		return nil, fmt.Errorf("clip has no image")
	}
	s.mu.Lock()
	dir := s.imagesDir
	s.mu.Unlock()
	if !fsutil.WithinDir(dir, c.ImagePath) {
		return nil, fmt.Errorf("image path is outside storage")
	}
	data, err := fsutil.ReadRegularFile(c.ImagePath, MaxImageBytes)
	if err != nil {
		return nil, err
	}
	if !IsPNG(data) {
		return nil, fmt.Errorf("stored image is not a PNG")
	}
	return data, nil
}

func (s *Store) AddImage(data []byte) (*Clip, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) > MaxImageBytes {
		return nil, ErrTooLarge
	}
	if !IsPNG(data) {
		return nil, fmt.Errorf("clipboard image is not a PNG")
	}

	hash := hashBytes(data, TypeImage)
	s.mu.Lock()
	imagesDir := s.imagesDir
	s.mu.Unlock()

	filename := hash[:16] + ".png"
	path := filepath.Join(imagesDir, filename)
	created := false
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, err
		}
		created = true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

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
		if created {
			_ = os.Remove(path)
		}
		return nil, err
	}
	s.removeClipFiles(dropped)
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
			prev := s.clips[i].Pinned
			s.clips[i].Pinned = !prev
			if err := s.saveLocked(); err != nil {
				s.clips[i].Pinned = prev
				return err
			}
			return nil
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
		snapshot := cloneClips(s.clips)
		s.clips = append(s.clips[:i], s.clips[i+1:]...)
		if err := s.saveLocked(); err != nil {
			s.clips = snapshot
			return err
		}
		s.removeClipFiles([]Clip{dropped})
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
	s.removeClipFiles(dropped)
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
