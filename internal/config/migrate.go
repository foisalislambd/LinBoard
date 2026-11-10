package config

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// legacyDataDir is the history location used before the XDG data directory split.
func legacyDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "linboard", "data"), nil
}

func migrateLegacyData(dest string) error {
	legacy, err := legacyDataDir()
	if err != nil {
		return err
	}
	legacy = filepath.Clean(legacy)
	dest = filepath.Clean(dest)
	if legacy == dest {
		return nil
	}
	legacyClips := filepath.Join(legacy, "clips.json")
	if _, err := os.Stat(legacyClips); err != nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dest, "clips.json")); err == nil {
		log.Printf("history: keeping %s (legacy copy still at %s)", dest, legacy)
		return nil
	}

	if _, err := os.Stat(dest); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.Rename(legacy, dest); err == nil {
			log.Printf("history: moved %s -> %s", legacy, dest)
			return nil
		}
	}

	if err := copyDir(legacy, dest); err != nil {
		return fmt.Errorf("migrate history: %w", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "clips.json")); err != nil {
		return fmt.Errorf("migrate history: clips.json missing after copy")
	}
	if err := os.RemoveAll(legacy); err != nil {
		log.Printf("history: copied to %s but could not remove %s: %v", dest, legacy, err)
		return nil
	}
	log.Printf("history: copied %s -> %s", legacy, dest)
	return nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// Do not retarget symlinks during migration; skip them.
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFileContents(path, target, info.Mode().Perm())
	})
}

func copyFileContents(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
