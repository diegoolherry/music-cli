package ui

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/diegoolherry/music-cli/internal/playback"
	"github.com/mattn/go-runewidth"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/diegoolherry/music-cli/internal/library"
)

type fakePlayer struct {
	current       string
	folder        string
	calls         []string
	fail          bool
	stopErr       error
	notifications chan error
}

func (p *fakePlayer) Play(folder string, files []string, selected string) error {
	p.calls = append(p.calls, "play:"+selected)
	if p.fail {
		return errors.New("device unavailable")
	}
	p.folder = folder
	p.current = selected
	return nil
}
func (p *fakePlayer) Current() string { return p.current }
func (p *fakePlayer) Next() error     { p.calls = append(p.calls, "next"); return nil }
func (p *fakePlayer) Previous() error { p.calls = append(p.calls, "previous"); return nil }
func (p *fakePlayer) Pause() error    { p.calls = append(p.calls, "pause"); return nil }
func (p *fakePlayer) Stop() error {
	p.calls = append(p.calls, "stop")
	p.current = ""
	return p.stopErr
}
func (p *fakePlayer) Errors() <-chan error { return p.notifications }
func press(m Model, key string) Model {
	next, _ := m.Update(tea.KeyPressMsg{Text: key, Code: map[string]rune{" ": ' '}[key]})
	return next.(Model)
}
func sample() library.Library {
	return library.Library{Playlists: []library.Playlist{{Name: "A", Tracks: []string{"01.mp3", "02.mp3"}}, {Name: "B", Tracks: []string{"other.mp3"}}}}
}

type fakeManager struct {
	calls []string
	err   error
}

func (f *fakeManager) CreatePlaylist(name string) error {
	f.calls = append(f.calls, "create:"+name)
	return f.err
}
func (f *fakeManager) RenamePlaylist(old, name string) error {
	f.calls = append(f.calls, "folder:"+old+":"+name)
	return f.err
}
func (f *fakeManager) RenameTrack(folder, file, base string) error {
	f.calls = append(f.calls, "track:"+folder+":"+file+":"+base)
	return f.err
}

func TestDialogsMutationAndRefresh(t *testing.T) {
	manager := &fakeManager{}
	scans := 0
	m := New("root", sample(), nil, &fakePlayer{})
	m.Manager = manager
	m.Scan = func(string) (library.Library, error) {
		scans++
		return library.Library{Playlists: []library.Playlist{{Name: "Z"}}}, nil
	}
	m = press(m, "c")
	if m.DialogInput != "" || m.DialogKind != "create" {
		t.Fatalf("create dialog: %+v", m)
	}
	for _, key := range []string{"n", "q", "x", "backspace", "enter"} {
		m = press(m, key)
	}
	if strings.Join(manager.calls, ",") != "create:nq" || scans != 1 || m.DialogKind != "" || m.FolderIndex != 0 || m.SelectedFolder != 0 {
		t.Fatalf("create/refresh: %+v %+v", m, manager)
	}
	m = press(m, "r")
	if m.DialogKind != "folder" || m.DialogInput != "Z" {
		t.Fatalf("folder dialog: %+v", m)
	}
	m = press(m, "esc")
	if len(manager.calls) != 1 {
		t.Fatal(manager.calls)
	}
	m.Library = sample()
	m = press(m, "tab")
	m = press(m, "r")
	if m.DialogKind != "track" || m.DialogInput != "01" {
		t.Fatalf("track dialog: %+v", m)
	}
	m = press(m, "backspace")
	m = press(m, "2")
	m = press(m, "enter")
	if manager.calls[1] != "track:A:01.mp3:02" || scans != 2 {
		t.Fatalf("track: %+v %+v", m, manager)
	}
}

func TestDialogErrorsAndKeyCapture(t *testing.T) {
	p := &fakePlayer{}
	manager := &fakeManager{err: errors.New("bad\x1b[2J")}
	m := New("root", sample(), nil, p)
	m.Manager = manager
	m.Scan = func(string) (library.Library, error) { return library.Library{}, errors.New("scan\x1b[2J") }
	m = press(m, "c")
	for _, key := range []string{"tab", "down", " ", "s", "enter"} {
		m = press(m, key)
	}
	if m.TracksFocused || m.FolderIndex != 0 || len(p.calls) != 0 || m.DialogKind == "" || strings.Contains(m.View().Content, "\x1b[2J") {
		t.Fatalf("dialog leaked: %+v", m)
	}
	manager.err = nil
	m = press(m, "enter")
	if m.DialogKind != "" || !strings.Contains(m.Feedback, "scan") || strings.Contains(m.View().Content, "\x1b[2J") {
		t.Fatalf("scan error: %+v", m)
	}
}

func TestQuitReleasesAndReportsFailure(t *testing.T) {
	p := &fakePlayer{current: "song.mp3", stopErr: errors.New("release failed")}
	m := press(New(`D:\Music`, sample(), nil, p), "q")
	if p.current != "" || !strings.Contains(m.Feedback, "release failed") {
		t.Fatalf("quit: %+v %+v", p, m)
	}
}
func TestCtrlCQuitsAndReleases(t *testing.T) {
	for _, tt := range []struct {
		name    string
		stopErr error
	}{
		{name: "successful release"},
		{name: "failed release", stopErr: errors.New("release failed")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := &fakePlayer{current: "song.mp3", stopErr: tt.stopErr}
			next, cmd := New(`D:\Music`, sample(), nil, p).Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			m := next.(Model)
			if len(p.calls) != 1 || p.calls[0] != "stop" || p.current != "" {
				t.Fatalf("stop calls = %v, current = %q", p.calls, p.current)
			}
			if cmd == nil || cmd() == nil {
				t.Fatal("expected tea.Quit command")
			}
			if m.QuitError != tt.stopErr {
				t.Fatalf("quit error = %v, want %v", m.QuitError, tt.stopErr)
			}
			if tt.stopErr != nil && !strings.Contains(m.Feedback, tt.stopErr.Error()) {
				t.Fatalf("feedback = %q", m.Feedback)
			}
		})
	}
}
func TestIdlePlaybackFailure(t *testing.T) {
	p := &fakePlayer{current: "song.mp3", notifications: make(chan error, 1)}
	m := New(`D:\Music`, sample(), nil, p)
	p.current = ""
	next, _ := m.Update(playbackNotice{err: errors.New("device failed")})
	m = next.(Model)
	if !strings.Contains(m.View().Content, "device failed") || !strings.Contains(m.View().Content, "Now playing: stopped") {
		t.Fatal(m.View().Content)
	}
}
func TestNavigationAndActiveQueue(t *testing.T) {
	p := &fakePlayer{}
	m := New(`D:\Music`, sample(), nil, p)
	m = press(m, "down")
	m = press(m, "down")
	if m.FolderIndex != 1 {
		t.Fatalf("folder bounds: %d", m.FolderIndex)
	}
	m = press(m, "enter")
	m = press(m, "tab")
	m = press(m, "enter")
	if p.current != "other.mp3" || !strings.Contains(p.folder, "B") {
		t.Fatalf("play: %+v", p)
	}
	m = press(m, "tab")
	m = press(m, "up")
	m = press(m, "enter")
	m = press(m, "n")
	m = press(m, "p")
	if p.current != "other.mp3" || p.calls[len(p.calls)-2] != "next" || p.calls[len(p.calls)-1] != "previous" {
		t.Fatalf("browse changed playback: %+v", p)
	}
	m = press(m, "tab")
	m = press(m, "down")
	m = press(m, "enter")
	if p.current != "02.mp3" {
		t.Fatalf("track selection: %+v", p)
	}
	m = press(m, " ")
	m = press(m, "s")
	if p.current != "" || !strings.Contains(strings.Join(p.calls, ","), "pause,stop") {
		t.Fatalf("controls: %+v", p)
	}
}

type quietBackend struct{ opened []string }
type quietStream struct{}

func (quietStream) Close() error { return nil }
func (b *quietBackend) Open(path string, _ func(error)) (playback.Stream, error) {
	b.opened = append(b.opened, path)
	return quietStream{}, nil
}
func (b *quietBackend) Pause(bool) error { return nil }

func TestBrowsingDoesNotReplaceEngineQueue(t *testing.T) {
	backend := &quietBackend{}
	engine := playback.New(backend)
	m := New(`D:\Music`, sample(), nil, engine)
	m = press(m, "tab")
	m = press(m, "enter")
	m = press(m, "tab")
	m = press(m, "down")
	m = press(m, "enter")
	m = press(m, "n")
	if engine.Current() != "02.mp3" || filepath.Base(filepath.Dir(backend.opened[len(backend.opened)-1])) != "A" {
		t.Fatalf("next escaped active queue: %v", backend.opened)
	}
	m = press(m, "p")
	if engine.Current() != "01.mp3" || filepath.Base(filepath.Dir(backend.opened[len(backend.opened)-1])) != "A" {
		t.Fatalf("previous escaped active queue: %v", backend.opened)
	}
}

func TestViewSanitizesAndFitsCells(t *testing.T) {
	for _, tt := range []struct {
		name  string
		model Model
	}{
		{"root error", New("bad\x1b[31m\x07", library.Library{}, errors.New("oops\x1b[2J\x07"), nil)},
		{"normal", New("bad\x1b[31m", library.Library{Playlists: []library.Playlist{{Name: "界界界界界界界界界界界界界界界界界界", Tracks: []string{"界.mp3"}}}}, nil, nil)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model
			m.Width = 10
			view := m.View().Content
			if strings.Count(view, "\x1b[") != 3 || strings.Contains(view, "\x07") {
				t.Fatalf("injected controls: %q", view)
			}
			for _, line := range strings.Split(view, "\n") {
				// Only the trusted heading uses styling codes.
				line = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(line, "\x1b[48;2;13;10;18m", ""), "\x1b[38;2;245;84;189m", ""), "\x1b[0m", "")
				if runewidth.StringWidth(line) > 10 {
					t.Fatalf("line exceeds width: %q", line)
				}
			}
		})
	}
}

func TestWideFilenameKeepsRightColumnAligned(t *testing.T) {
	m := New("root", library.Library{Playlists: []library.Playlist{{Name: "界", Tracks: []string{"song.mp3"}}, {Name: "plain"}}}, nil, nil)
	m.Width = 60
	lines := strings.Split(m.View().Content, "\n")
	if strings.Index(lines[1], "Tracks") < 0 || runewidth.StringWidth(strings.Split(lines[1], "Tracks")[0]) != runewidth.StringWidth(strings.Split(lines[2], "song.mp3")[0]) {
		t.Fatalf("columns shifted: %q", lines)
	}
}

func TestPaneLayoutRespondsToWidth(t *testing.T) {
	for _, tt := range []struct {
		name       string
		width      int
		sideBySide bool
	}{
		{"default", 0, true},
		{"wide", 80, true},
		{"narrow", 24, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := New(`D:\Music`, sample(), nil, nil)
			m.SelectedFolder = 1
			if tt.width != 0 {
				next, _ := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: 20})
				m = next.(Model)
			}
			content := m.View().Content
			lines := strings.Split(content, "\n")
			headingRow, trackRow := -1, -1
			for i, line := range lines {
				if strings.Contains(line, "Folders") {
					headingRow = i
				}
				if strings.Contains(line, "Tracks") && trackRow == -1 {
					trackRow = i
				}
			}
			if headingRow < 0 || trackRow < 0 || (headingRow == trackRow) != tt.sideBySide {
				t.Fatalf("unexpected pane headings at width %d: %q", tt.width, content)
			}
			if tt.sideBySide && strings.Index(lines[headingRow], "Tracks") < strings.Index(lines[headingRow], "Folders")+20 {
				t.Fatalf("tracks heading not in right column: %q", lines[headingRow])
			}
			if !strings.Contains(content, "other.mp3") {
				t.Fatalf("selected track missing: %q", content)
			}
		})
	}
}
func TestEmptyErrorAndUnsupported(t *testing.T) {
	p := &fakePlayer{fail: true}
	m := New(`D:\Music`, library.Library{}, errors.New(`D:\Music unavailable`), p)
	if !strings.Contains(m.View().Content, "D:\\Music") || !strings.Contains(m.View().Content, "restore access") {
		t.Fatal(m.View().Content)
	}
	m = press(m, "enter")
	m = press(m, "c")
	if len(p.calls) != 0 {
		t.Fatal(p.calls)
	}
	m = New(`D:\Music`, sample(), nil, p)
	m = press(m, "tab")
	m = press(m, "enter")
	if !strings.Contains(m.View().Content, "device unavailable") || p.current != "" {
		t.Fatal(m.View().Content)
	}
	if !strings.Contains(m.View().Content, " c create ") || strings.Contains(m.View().Content, " x ") {
		t.Fatal("incorrect controls advertised")
	}
}
