package manage

import (
	"os"
	"path/filepath"

	"github.com/diegoolherry/music-cli/internal/playback"
	"strings"
	"testing"
)

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
			if err := service.RenameTrack("Rock", "Song.mp3", "Blocked"); err == nil || !strings.Contains(err.Error(), "active") {
				t.Fatalf("active rename error = %v", err)
			}
			if _, err := os.Stat(filepath.Join(folder, "Song.mp3")); err != nil {
				t.Fatalf("active track changed: %v", err)
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
	if err := service.RenameTrack("Rock", "Other.mp3", "OtherNew"); err != nil {
		t.Fatalf("nonactive rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "OtherNew.mp3")); err != nil {
		t.Fatalf("nonactive target: %v", err)
	}
	if err := engine.Stop(); err != nil {
		t.Fatal(err)
	}
	if !backend.stream.closed || engine.ActivePath() != "" {
		t.Fatal("Stop did not release active stream")
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
