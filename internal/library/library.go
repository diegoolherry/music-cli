// Package library discovers immediate playlists and tracks without changing the filesystem.
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegoolherry/music-cli/internal/playback"
)

// Playlist contains only directly contained, regular MP3 filenames.
type Playlist struct {
	Name   string
	Tracks []string
}

// Library retains partial-read warnings alongside accessible playlists.
type Library struct {
	Playlists []Playlist
	Warnings  []error
}

type filesystem struct {
	read func(string) ([]os.DirEntry, error)
	stat func(string) (os.FileInfo, error)
}

// Scan reads root without creating or changing it. A root failure returns no library.
func Scan(root string) (Library, error) {
	return scan(root, filesystem{read: os.ReadDir, stat: os.Lstat})
}

func scan(root string, fs filesystem) (Library, error) {
	var result Library
	info, err := fs.stat(root)
	if err != nil {
		return result, rootError(root, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, rootError(root, fmt.Errorf("not a real directory"))
	}
	entries, err := fs.read(root)
	if err != nil {
		return result, rootError(root, err)
	}

	// Order directory names by the same case-insensitive, deterministic comparator as playback.
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	for _, name := range playback.Order(names) {
		path := filepath.Join(root, name)
		info, err := fs.stat(path)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Errorf("playlist entry %q: %w", path, err))
			continue
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		playlist := Playlist{Name: name}
		children, err := fs.read(path)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Errorf("playlist %q: %w", path, err))
		}
		for _, child := range children {
			if !strings.EqualFold(filepath.Ext(child.Name()), ".mp3") {
				continue
			}
			trackPath := filepath.Join(path, child.Name())
			childInfo, err := fs.stat(trackPath)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Errorf("track %q: %w", trackPath, err))
				continue
			}
			if childInfo.Mode().IsRegular() {
				playlist.Tracks = append(playlist.Tracks, child.Name())
			}
		}
		playlist.Tracks = playback.Order(playlist.Tracks)
		result.Playlists = append(result.Playlists, playlist)
	}
	return result, nil
}

func rootError(root string, err error) error {
	return fmt.Errorf("library root %s unavailable: create or restore access to this directory: %w", root, err)
}
