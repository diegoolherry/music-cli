// Package manage performs bounded, non-destructive library name changes.
package manage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Service manages immediate playlists under Root. ActivePath returns the open
// playback stream path, including while paused; nil means no active playback.
type Service struct {
	Root       string
	ActivePath func() string
}

func New(root string, activePath func() string) *Service {
	return &Service{Root: root, ActivePath: activePath}
}

func (s *Service) activePath() string {
	if s.ActivePath == nil {
		return ""
	}
	return s.ActivePath()
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func validName(name string) error {
	if name == "" || strings.TrimSpace(name) == "" || name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("invalid name %q", name)
	}
	for _, r := range name {
		if r < 32 || r == 127 || strings.ContainsRune("<>:\"/\\|?*", r) {
			return fmt.Errorf("invalid name %q", name)
		}
	}
	stem := strings.SplitN(name, ".", 2)[0]
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "COM¹", "COM²", "COM³", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9", "LPT¹", "LPT²", "LPT³":
		return fmt.Errorf("reserved name %q", name)
	}
	return nil
}
func directory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("directory %q unavailable: %w", path, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q is not a real directory", path)
	}
	return nil
}
func (s *Service) root() error {
	if s.Root == "" {
		return fmt.Errorf("library root is empty")
	}
	if err := directory(s.Root); err != nil {
		return fmt.Errorf("library root: %w", err)
	}
	return nil
}
func (s *Service) playlist(name string) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}
	if err := s.root(); err != nil {
		return "", err
	}
	path := filepath.Join(s.Root, name)
	if err := directory(path); err != nil {
		return "", fmt.Errorf("playlist: %w", err)
	}
	return path, nil
}
func available(parent, name string) error {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return fmt.Errorf("read %q: %w", parent, err)
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), name) {
			return fmt.Errorf("name %q already exists", name)
		}
	}
	return nil
}
func (s *Service) CreatePlaylist(name string) error {
	if err := validName(name); err != nil {
		return err
	}
	if err := s.root(); err != nil {
		return err
	}
	if err := available(s.Root, name); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(s.Root, name), 0755); err != nil {
		return fmt.Errorf("create playlist: %w", err)
	}
	return nil
}
func (s *Service) RenamePlaylist(oldName, newName string) error {
	old, err := s.playlist(oldName)
	if err != nil {
		return err
	}
	if err := validName(newName); err != nil {
		return err
	}
	if active := s.activePath(); active != "" && samePath(filepath.Dir(active), old) {
		return fmt.Errorf("playlist %q is active; stop playback before renaming", old)
	}
	if err := available(s.Root, newName); err != nil {
		return err
	}
	if err := os.Rename(old, filepath.Join(s.Root, newName)); err != nil {
		return fmt.Errorf("rename playlist: %w", err)
	}
	return nil
}
func (s *Service) RenameTrack(folder, file, base string) error {
	parent, err := s.playlist(folder)
	if err != nil {
		return err
	}
	if err := validName(file); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Ext(file), ".mp3") {
		return fmt.Errorf("track %q is not an MP3", file)
	}
	if err := validName(base); err != nil {
		return err
	}
	old := filepath.Join(parent, file)
	info, err := os.Lstat(old)
	if err != nil {
		return fmt.Errorf("track %q unavailable: %w", old, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("track %q is not a regular MP3", old)
	}
	if active := s.activePath(); active != "" && samePath(active, old) {
		return fmt.Errorf("track %q is active; stop playback before renaming", old)
	}
	target := base + ".mp3"
	if err := available(parent, target); err != nil {
		return err
	}
	if err := os.Rename(old, filepath.Join(parent, target)); err != nil {
		return fmt.Errorf("rename track: %w", err)
	}
	return nil
}
