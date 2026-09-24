package library

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestScanFilesystem(t *testing.T) {
	root := t.TempDir()
	makeFile := func(parts ...string) {
		t.Helper()
		p := filepath.Join(append([]string{root}, parts...)...)
		if err := os.WriteFile(p, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{"rock", "Rock2", "rock/live", "empty"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"b.MP3", "A.mp3", "10.mp3", "02.mp3", "01.mp3", "notes.txt", "live/nested.mp3"} {
		makeFile("rock", name)
	}
	makeFile("root.mp3")
	got, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings: %v", got.Warnings)
	}
	want := []Playlist{{Name: "empty"}, {Name: "rock", Tracks: []string{"01.mp3", "02.mp3", "10.mp3", "A.mp3", "b.MP3"}}, {Name: "Rock2"}}
	if !reflect.DeepEqual(got.Playlists, want) {
		t.Fatalf("got %#v, want %#v", got.Playlists, want)
	}
}

func TestMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	_, err := Scan(root)
	if err == nil || !strings.Contains(err.Error(), root) || !strings.Contains(err.Error(), "create or restore access") {
		t.Fatalf("error: %v", err)
	}
}

func TestEmptyRoot(t *testing.T) {
	got, err := Scan(t.TempDir())
	if err != nil || len(got.Playlists) != 0 || len(got.Warnings) != 0 {
		t.Fatalf("result: %+v, %v", got, err)
	}
}

func TestPartialFailures(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"good", "bad"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"good/ok.mp3", "good/broken.mp3"} {
		if err := os.WriteFile(filepath.Join(root, file), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ops := filesystem{read: os.ReadDir, stat: os.Lstat}
	ops.read = func(path string) ([]os.DirEntry, error) {
		if path == filepath.Join(root, "bad") {
			return nil, errors.New("denied")
		}
		return os.ReadDir(path)
	}
	ops.stat = func(path string) (os.FileInfo, error) {
		if path == filepath.Join(root, "good", "broken.mp3") {
			return nil, errors.New("denied")
		}
		return os.Lstat(path)
	}
	got, err := scan(root, ops)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Playlists) != 2 || !reflect.DeepEqual(got.Playlists[1].Tracks, []string{"ok.mp3"}) {
		t.Fatalf("playlists: %+v", got.Playlists)
	}
	if len(got.Warnings) != 2 || !strings.Contains(got.Warnings[0].Error()+got.Warnings[1].Error(), "broken.mp3") || !strings.Contains(got.Warnings[0].Error()+got.Warnings[1].Error(), "bad") {
		t.Fatalf("warnings: %v", got.Warnings)
	}
}

func TestPlaylistPartialReadRetainsTracks(t *testing.T) {
	root := t.TempDir()
	playlist := filepath.Join(root, "partial")
	if err := os.Mkdir(playlist, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(playlist, "visible.mp3"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	readErr := errors.New("partial read")
	ops := filesystem{stat: os.Lstat, read: func(path string) ([]os.DirEntry, error) {
		entries, err := os.ReadDir(path)
		if err == nil && path == playlist {
			return entries, readErr
		}
		return entries, err
	}}
	got, err := scan(root, ops)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Playlists, []Playlist{{Name: "partial", Tracks: []string{"visible.mp3"}}}) {
		t.Fatalf("playlists: %+v", got.Playlists)
	}
	if len(got.Warnings) != 1 || !errors.Is(got.Warnings[0], readErr) {
		t.Fatalf("warnings: %v", got.Warnings)
	}
}

func TestRootReadFailure(t *testing.T) {
	root := t.TempDir()
	_, err := scan(root, filesystem{read: func(string) ([]os.DirEntry, error) { return nil, errors.New("denied") }, stat: os.Lstat})
	if err == nil || !strings.Contains(err.Error(), root) {
		t.Fatalf("error: %v", err)
	}
}

func TestSymlinksIgnored(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.mp3"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "safe"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.mp3"), filepath.Join(root, "safe", "linked.mp3")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	got, err := Scan(root)
	if err != nil || !reflect.DeepEqual(got.Playlists, []Playlist{{Name: "safe"}}) {
		t.Fatalf("result: %+v, %v", got, err)
	}
	_, err = Scan(filepath.Join(root, "linked"))
	if err == nil {
		t.Fatal("symlink root accepted")
	}
}
