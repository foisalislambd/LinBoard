package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/foisal/linboard/internal/config"
)

type diskData struct {
	NextID int64  `json:"next_id"`
	Clips  []Clip `json:"clips"`
}

// fileStore is LinBoard's native JSON history backend (no SQLite).
type fileStore struct {
	mu        sync.Mutex
	path      string
	imagesDir string
	maxItems  int
	nextID    int64
	clips     []Clip
}

func openFileStore(maxItems int) (*fileStore, error) {
	dataDir, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	imagesDir, err := config.ImagesDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return nil, err
	}

	path := filepath.Join(dataDir, "clips.json")
	s := &fileStore{
		path:      path,
		imagesDir: imagesDir,
		maxItems:  maxItems,
	}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *fileStore) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var data diskData
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	s.nextID = data.NextID
	s.clips = data.Clips
	for _, c := range s.clips {
		if c.ID >= s.nextID {
			s.nextID = c.ID + 1
		}
	}
	return nil
}

func (s *fileStore) saveLocked() error {
	data := diskData{NextID: s.nextID, Clips: s.clips}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *fileStore) findByHash(hash string) (int, *Clip) {
	for i := range s.clips {
		if s.clips[i].Hash == hash {
			return i, &s.clips[i]
		}
	}
	return -1, nil
}

func (s *fileStore) findByID(id int64) *Clip {
	for i := range s.clips {
		if s.clips[i].ID == id {
			return &s.clips[i]
		}
	}
	return nil
}

func (s *fileStore) sortedIndices() []int {
	indices := make([]int, len(s.clips))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		return clipLess(&s.clips[indices[i]], &s.clips[indices[j]])
	})
	return indices
}

func clipLess(a, b *Clip) bool {
	if a.Pinned != b.Pinned {
		return a.Pinned
	}
	return a.CreatedAt.After(b.CreatedAt)
}
