package store

import (
	"encoding/json"
	"log"
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
	if err := os.MkdirAll(imagesDir, 0o700); err != nil {
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
	if dropped := s.pruneInMemory(); len(dropped) > 0 {
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
		removeClipFiles(dropped)
	}
	s.cleanupOrphanImages()
	return s, nil
}

func (s *fileStore) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var data diskData
	if err := json.Unmarshal(b, &data); err != nil {
		bak := s.path + ".corrupt"
		if renErr := os.Rename(s.path, bak); renErr != nil {
			return err
		}
		log.Printf("clips.json is unreadable — moved aside to %s (%v)", bak, err)
		s.nextID = 0
		s.clips = nil
		return nil
	}
	s.nextID = data.NextID
	s.clips = data.Clips
	if s.clips == nil {
		s.clips = []Clip{}
	}
	for _, c := range s.clips {
		if c.ID >= s.nextID {
			s.nextID = c.ID + 1
		}
	}
	return nil
}

func (s *fileStore) saveLocked() error {
	data := diskData{NextID: s.nextID, Clips: s.clips}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(b)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tmp)
		return writeErr
	}
	if syncErr != nil {
		_ = os.Remove(tmp)
		return syncErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, s.path); err != nil {
		// Windows cannot rename over an existing file; Linux replace is atomic.
		_ = os.Remove(s.path)
		if err2 := os.Rename(tmp, s.path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}
	return nil
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

// pruneInMemory drops oldest unpinned clips. Files are not deleted; caller must
// persist first, then removeClipFiles so a failed save cannot lose data.
func (s *fileStore) pruneInMemory() []Clip {
	if s.maxItems < 1 || len(s.clips) <= s.maxItems {
		return nil
	}
	indices := s.sortedIndices()
	keep := make(map[int64]bool, s.maxItems)
	pinnedN := 0
	for _, idx := range indices {
		c := s.clips[idx]
		if !c.Pinned {
			continue
		}
		keep[c.ID] = true
		pinnedN++
	}
	remain := s.maxItems - pinnedN
	if remain < 0 {
		remain = 0
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
	out := make([]Clip, 0, len(keep))
	var dropped []Clip
	for i := range s.clips {
		c := s.clips[i]
		if keep[c.ID] {
			out = append(out, c)
		} else {
			dropped = append(dropped, c)
		}
	}
	s.clips = out
	return dropped
}

func removeClipFiles(dropped []Clip) {
	for _, c := range dropped {
		if c.ImagePath != "" {
			_ = os.Remove(c.ImagePath)
		}
	}
}

func (s *fileStore) cleanupOrphanImages() {
	entries, err := os.ReadDir(s.imagesDir)
	if err != nil {
		return
	}
	used := make(map[string]struct{}, len(s.clips))
	for _, c := range s.clips {
		if c.ImagePath != "" {
			used[filepath.Base(c.ImagePath)] = struct{}{}
		}
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, ok := used[e.Name()]; ok {
			continue
		}
		_ = os.Remove(filepath.Join(s.imagesDir, e.Name()))
	}
}
