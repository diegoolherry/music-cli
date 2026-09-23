package manage

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/diegoolherry/music-cli/internal/playback"
	"strings"
	"testing"
)

func TestRecyclePlaylistBoundary(t *testing.T) {
	root, s := fixture(t)
	target := filepath.Join(root, "Rock")
	calls := 0
	s.Recycler = func(path string) error {
		calls++
		if filepath.Base(path) != "Rock" || filepath.Dir(filepath.Dir(path)) != root || filepath.Dir(path) == root {
			t.Fatalf("staged path = %q, root %q", path, root)
		}
		return os.ErrPermission
	}
	for _, name := range []string{"..", "../Rock", ".", "missing"} {
		if err := s.RecyclePlaylist(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid calls: %d", calls)
	}
	s.ActivePath = func() string { return filepath.Join(target, "Song.mp3") }
	if err := s.RecyclePlaylist("Rock"); err == nil {
		t.Fatal("active playlist accepted")
	}
	if calls != 0 {
		t.Fatal("recycler called while active")
	}
	other := filepath.Join(root, "Other")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	s.ActivePath = func() string { return filepath.Join(other, "track.mp3") }
	if err := s.RecyclePlaylist("Rock"); err == nil {
		t.Fatal("fake failure hidden for other active folder")
	}
	if calls != 1 {
		t.Fatalf("other-folder calls: %d", calls)
	}
	s.ActivePath = nil // Stop releases the active path.
	if err := s.RecyclePlaylist("Rock"); err == nil {
		t.Fatal("fake failure hidden")
	}
	if calls != 2 {
		t.Fatalf("calls: %d", calls)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("source lost: %v", err)
	}
	link := filepath.Join(root, "Link")
	if err := os.Symlink(target, link); err == nil {
		if err := s.RecyclePlaylist("Link"); err == nil {
			t.Fatal("symlink accepted")
		}
	}
}

func TestRecycleRestoreOccupiedAfterCheck(t *testing.T) {
	root, s := fixture(t)
	cause := errors.New("recycler failed")
	var staged string
	s.Recycler = func(path string) error { staged = path; return cause }
	s.beforeRestore = func() {
		original := filepath.Join(root, "Rock")
		if err := os.Mkdir(original, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(original, "marker"), []byte("new"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	err := s.RecyclePlaylist("Rock")
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "restore playlist") {
		t.Fatalf("error = %v", err)
	}
	marker, readErr := os.ReadFile(filepath.Join(root, "Rock", "marker"))
	if readErr != nil || string(marker) != "new" {
		t.Fatalf("occupied destination changed: %q, %v", marker, readErr)
	}
	song, readErr := os.ReadFile(filepath.Join(staged, "Song.mp3"))
	if readErr != nil || string(song) != "song" {
		t.Fatalf("staged payload changed: %q, %v", song, readErr)
	}
}

func TestRecycleCaptureReplacementAndSuccess(t *testing.T) {
	t.Run("replacement is never recycled", func(t *testing.T) {
		root, s := fixture(t)
		original := filepath.Join(root, "Rock")
		displaced := filepath.Join(root, "Displaced")
		s.beforeCapture = func() {
			if err := os.Rename(original, displaced); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(original, 0700); err != nil {
				t.Fatal(err)
			}
		}
		s.Recycler = func(string) error { t.Fatal("recycled replacement"); return nil }
		if err := s.RecyclePlaylist("Rock"); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
			t.Fatalf("error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(displaced, "Song.mp3")); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected restored replacement and displaced original: %v", entries)
		}
	})
	t.Run("success moves staged payload", func(t *testing.T) {
		root, s := fixture(t)
		outside := filepath.Join(t.TempDir(), "recycled")
		s.Recycler = func(path string) error {
			if filepath.Base(path) != "Rock" || filepath.Dir(filepath.Dir(path)) != root {
				t.Fatalf("path = %q", path)
			}
			return os.Rename(path, outside)
		}
		if err := s.RecyclePlaylist("Rock"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(outside, "Song.mp3")); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("staging residue: %v, %v", entries, err)
		}
	})
	t.Run("success without removal is rejected", func(t *testing.T) {
		root, s := fixture(t)
		s.Recycler = func(string) error { return nil }
		if err := s.RecyclePlaylist("Rock"); err == nil {
			t.Fatal("false success")
		}
		if info, err := os.Stat(filepath.Join(root, "Rock")); err != nil || !info.IsDir() {
			t.Fatalf("original playlist not restored: %v, %v", info, err)
		}
		if _, err := os.Stat(filepath.Join(root, "Rock", "Song.mp3")); err != nil {
			t.Fatalf("original song not restored: %v", err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 1 || entries[0].Name() != "Rock" {
			t.Fatalf("staging residue or missing original: %v, %v", entries, err)
		}
	})
	t.Run("failure restores contents", func(t *testing.T) {
		root, s := fixture(t)
		s.Recycler = func(string) error { return os.ErrPermission }
		if err := s.RecyclePlaylist("Rock"); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, "Rock", "Song.mp3")); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 1 {
			t.Fatalf("staging residue: %v, %v", entries, err)
		}
	})
}

func fixture(t *testing.T) (string, *Service) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Rock"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Rock", "Song.mp3"), []byte("song"), 0600); err != nil {
		t.Fatal(err)
	}
	return root, New(root, nil)
}
func TestReservedWindowsDeviceNames(t *testing.T) {
	for _, name := range []string{"COM¹", "com².txt", "COM³.mp3", "LPT¹", "lpt².txt", "LPT³.mp3", "CONIN$", "conin$.txt", "CONOUT$", "conout$.mp3"} {
		t.Run(name, func(t *testing.T) {
			root, s := fixture(t)
			if err := s.CreatePlaylist(name); err == nil {
				t.Fatalf("CreatePlaylist accepted %q", name)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "Rock" {
				t.Fatalf("playlist directory changed for %q: %v", name, entries)
			}
			if err := s.RenameTrack("Rock", "Song.mp3", name); err == nil {
				t.Fatalf("RenameTrack accepted base %q", name)
			}
			if _, err := os.Stat(filepath.Join(root, "Rock", "Song.mp3")); err != nil {
				t.Fatalf("original track changed: %v", err)
			}
		})
	}
}

// manageBackend keeps the playback stream lifecycle real without opening audio.
type manageBackend struct{ stream *manageStream }
type manageStream struct{ closed bool }

func (s *manageStream) Close() error { s.closed = true; return nil }
func (b *manageBackend) Open(_ string, _ func(error)) (playback.Stream, error) {
	b.stream = &manageStream{}
	return b.stream, nil
}
func (*manageBackend) Pause(bool) error { return nil }

func TestRenameTrackWithPlaybackEngine(t *testing.T) {
	root, _ := fixture(t)
	folder := filepath.Join(root, "Rock")
	other := filepath.Join(folder, "Other.mp3")
	if err := os.WriteFile(other, []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	backend := &manageBackend{}
	engine := playback.New(backend)
	service := New(root, engine.ActivePath)
	if err := engine.Play(folder, []string{"Song.mp3", "Other.mp3"}, "Song.mp3"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		pause bool
	}{
		{name: "playing"},
		{name: "paused", pause: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.pause {
				if err := engine.Pause(); err != nil {
					t.Fatal(err)
				}
			}
			for _, track := range []string{"Song.mp3", "Other.mp3"} {
				if err := service.RenameTrack("Rock", track, "Blocked"); err == nil || !strings.Contains(err.Error(), "active") {
					t.Fatalf("rename %q during %s error = %v", track, tc.name, err)
				}
				if _, err := os.Stat(filepath.Join(folder, track)); err != nil {
					t.Fatalf("queued track %q changed: %v", track, err)
				}
			}
			if _, err := os.Stat(filepath.Join(folder, "Blocked.mp3")); !os.IsNotExist(err) {
				t.Fatalf("blocked target exists: %v", err)
			}
		})
	}
	if err := service.RenamePlaylist("Rock", "Moved"); err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("active playlist rename error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "Song.mp3")); err != nil {
		t.Fatalf("active playlist changed: %v", err)
	}
	if err := service.CreatePlaylist("Jazz"); err != nil {
		t.Fatal(err)
	}
	if err := service.RenamePlaylist("Jazz", "Blues"); err != nil {
		t.Fatalf("inactive playlist rename: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Blues", "Elsewhere.mp3"), []byte("other folder"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.RenameTrack("Blues", "Elsewhere.mp3", "OtherNew"); err != nil {
		t.Fatalf("other-folder rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Blues", "OtherNew.mp3")); err != nil {
		t.Fatalf("other-folder target: %v", err)
	}
	if err := engine.Stop(); err != nil {
		t.Fatal(err)
	}
	if !backend.stream.closed || engine.ActivePath() != "" {
		t.Fatal("Stop did not release active stream")
	}
	if err := service.RenameTrack("Rock", "Other.mp3", "QueuedReleased"); err != nil {
		t.Fatalf("queued rename after Stop: %v", err)
	}
	if err := service.RenameTrack("Rock", "Song.mp3", "Released"); err != nil {
		t.Fatalf("rename after Stop: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "Released.mp3")); err != nil {
		t.Fatalf("released target: %v", err)
	}
	if err := service.RenamePlaylist("Rock", "Moved"); err != nil {
		t.Fatalf("rename playlist after Stop: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Moved", "Released.mp3")); err != nil {
		t.Fatalf("moved playlist track: %v", err)
	}
}

func TestManage(t *testing.T) {
	t.Run("create and rename", func(t *testing.T) {
		root, s := fixture(t)
		if err := s.CreatePlaylist("Jazz"); err != nil {
			t.Fatal(err)
		}
		if err := s.RenamePlaylist("Jazz", "Blues"); err != nil {
			t.Fatal(err)
		}
		if err := s.RenameTrack("Rock", "Song.mp3", "New"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(root, "Rock", "New.mp3")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("invalid and duplicate names leave files intact", func(t *testing.T) {
		root, s := fixture(t)
		for _, name := range []string{"", " ", "..", "bad/name", "bad\\name", "bad:name", "bad?name", "bad*name", "bad|name", "bad\"name", "bad<name", "bad>name", "bad.", "CON", "con.txt", "LPT1", "a\x01b", "rock"} {
			t.Run(name, func(t *testing.T) {
				if err := s.CreatePlaylist(name); err == nil {
					t.Fatal("accepted invalid name")
				}
				if _, err := os.Stat(filepath.Join(root, "Rock", "Song.mp3")); err != nil {
					t.Fatal(err)
				}
			})
		}
		if err := s.RenamePlaylist("Rock", "rock"); err == nil {
			t.Fatal("case collision")
		}
		if err := s.RenameTrack("Rock", "Song.mp3", "song"); err == nil {
			t.Fatal("case collision")
		}
		if err := os.WriteFile(filepath.Join(root, "Rock", "Other.MP3"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := s.RenameTrack("Rock", "Song.mp3", "other"); err == nil {
			t.Fatal("duplicate track")
		}
	})
	t.Run("scope and symlinks", func(t *testing.T) {
		root, s := fixture(t)
		os.Mkdir(filepath.Join(root, "Rock", "Nested"), 0700)
		os.WriteFile(filepath.Join(root, "Rock", "Nested", "Deep.mp3"), nil, 0600)
		os.WriteFile(filepath.Join(root, "Rock", "Note.txt"), nil, 0600)
		for _, tc := range []struct{ folder, track string }{{"..", "Song.mp3"}, {"Rock/Nested", "Deep.mp3"}, {"Rock", "Note.txt"}, {"Rock", "Nested"}} {
			if err := s.RenameTrack(tc.folder, tc.track, "New"); err == nil {
				t.Fatalf("accepted %v", tc)
			}
		}
		if err := s.RenamePlaylist(".", "New"); err == nil {
			t.Fatal("root rename")
		}
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "Link")); err == nil {
			if err := s.RenamePlaylist("Link", "Moved"); err == nil {
				t.Fatal("renamed symlink")
			}
			if err := s.CreatePlaylist("Link"); err == nil {
				t.Fatal("duplicate symlink")
			}
		}
		if err := os.Symlink(filepath.Join(root, "Rock", "Song.mp3"), filepath.Join(root, "Rock", "Link.mp3")); err == nil {
			if err := s.RenameTrack("Rock", "Link.mp3", "Moved"); err == nil {
				t.Fatal("renamed symlink track")
			}
		}
	})
	t.Run("active until stopped", func(t *testing.T) {
		root, _ := fixture(t)
		active := true
		s := New(root, func() string {
			if active {
				return filepath.Join(root, "Rock", "Song.mp3")
			}
			return ""
		})
		if err := s.RenameTrack("Rock", "Song.mp3", "New"); err == nil || !strings.Contains(err.Error(), "active") {
			t.Fatalf("missing active error: %v", err)
		}
		active = false
		if err := s.RenameTrack("Rock", "Song.mp3", "New"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("missing root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "missing")
		if err := New(root, nil).CreatePlaylist("A"); err == nil {
			t.Fatal("created root")
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("root changed: %v", err)
		}
	})
}
