package ui

import (
	"errors"
	"fmt"
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

func (f *fakeManager) RecyclePlaylist(name string) error {
	f.calls = append(f.calls, "recycle:"+name)
	return f.err
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

func TestRecycleConfirmation(t *testing.T) {
	manager := &fakeManager{}
	p := &fakePlayer{}
	m := New("root", sample(), nil, p)
	m.Manager = manager
	scans := 0
	m.Scan = func(string) (library.Library, error) {
		scans++
		return library.Library{Playlists: []library.Playlist{{Name: "B"}}}, nil
	}
	m = press(m, "x")
	if m.DialogKind != "recycle" || !strings.Contains(m.View().Content, "A") || !strings.Contains(m.View().Content, "Recycle Bin") || !strings.Contains(m.View().Content, "Cancel") {
		t.Fatalf("dialog: %+v %q", m, m.View().Content)
	}
	for _, key := range []string{"x", "q", "tab", "n", "s", " ", "backspace"} {
		m = press(m, key)
	}
	if m.DialogKind != "recycle" || m.TracksFocused || len(p.calls) != 0 || len(manager.calls) != 0 {
		t.Fatalf("key leak: %+v", m)
	}
	m = press(m, "enter")
	if m.DialogKind != "" || len(manager.calls) != 0 {
		t.Fatalf("default cancel: %+v", m)
	}
	m = press(m, "x")
	m = press(m, "down")
	m = press(m, "enter")
	if strings.Join(manager.calls, ",") != "recycle:A" || scans != 1 || m.DialogKind != "" || len(m.Library.Playlists) != 1 || m.Library.Playlists[0].Name != "B" || m.FolderIndex != 0 || m.SelectedFolder != 0 || m.Feedback != "" {
		t.Fatalf("move: %+v %+v", m, manager)
	}
}

func TestRecycleRepeatedEnterAfterFailure(t *testing.T) {
	manager := &fakeManager{err: errors.New("failed")}
	m := New("root", sample(), nil, nil)
	m.Manager = manager
	m = press(press(m, "x"), "down")
	m = press(m, "enter")
	if m.DialogKind != "recycle" || m.recycleMove || !strings.Contains(m.View().Content, "[Cancel]") || !strings.Contains(m.View().Content, "Error: failed") {
		t.Fatalf("failure should remain visible with Cancel selected: %+v %q", m, m.View().Content)
	}
	m = press(m, "enter")
	if m.DialogKind != "" || m.Feedback != "" || strings.Join(manager.calls, ",") != "recycle:A" {
		t.Fatalf("second Enter should deliberately cancel without retry: %+v %+v", m, manager)
	}
}

func TestRecycleRetryRequiresVisibleCompactAction(t *testing.T) {
	manager := &fakeManager{err: errors.New("failed")}
	m := New("root", sample(), nil, nil)
	m.Manager = manager
	m = press(press(m, "x"), "down")
	m = press(m, "enter")
	if len(manager.calls) != 1 || m.DialogKind != "recycle" {
		t.Fatalf("expected initial failed move: %+v %+v", m, manager)
	}
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 2})
	m = resized.(Model)
	m = press(m, "down")
	m = press(m, "enter")
	if len(manager.calls) != 1 || strings.Contains(m.View().Content, "[Move]") {
		t.Fatalf("hidden move must not retry: %+v %q", manager, m.View().Content)
	}
	resized, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 8})
	m = resized.(Model)
	if m.recycleMove || !m.recycleConfirmationFits() {
		t.Fatalf("resize must restore fresh choice: %+v", m)
	}
	m = press(press(m, "down"), "enter")
	if len(manager.calls) != 2 {
		t.Fatalf("visible deliberate selection should retry: %+v", manager)
	}
}

func TestRecycleFailureAndGuards(t *testing.T) {
	manager := &fakeManager{err: errors.New("failed\x1b[2J")}
	m := New("root", sample(), nil, nil)
	m.Manager = manager
	m.Scan = func(string) (library.Library, error) { return library.Library{}, errors.New("scan\x1b[2J") }
	m = press(press(press(m, "x"), "up"), "enter")
	if m.DialogKind != "recycle" || !strings.Contains(m.Feedback, "failed") || strings.Contains(m.View().Content, "\x1b[2J") {
		t.Fatalf("failure: %+v", m)
	}
	m = press(m, "esc")
	if len(manager.calls) != 1 {
		t.Fatal(manager.calls)
	}
	manager.err = nil
	m = press(press(press(m, "x"), "down"), "enter")
	if m.DialogKind != "" || !strings.Contains(m.Feedback, "scan") || strings.Contains(m.View().Content, "\x1b[2J") || m.Library.Playlists[0].Name != "A" {
		t.Fatalf("scan failure: %+v", m)
	}
	for _, guarded := range []Model{New("root", sample(), errors.New("root unavailable"), nil), New("root", library.Library{}, nil, nil)} {
		guarded.Manager = manager
		if press(guarded, "x").DialogKind != "" {
			t.Fatal("unsafe dialog")
		}
	}
	m = New("root", sample(), nil, nil)
	m.Manager = manager
	m.TracksFocused = true
	if press(m, "x").DialogKind != "" {
		t.Fatal("track pane dialog")
	}
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

func TestMagentaLayoutStructure(t *testing.T) {
	p := &fakePlayer{}
	m := New(`D:\Music`, sample(), nil, p)
	content := m.View().Content
	for _, want := range []string{`D:\Music`, "FOLDERS / PLAYLISTS", "TRACKS", "2 PLAYLISTS", "2 TRACKS", "A", "B", "01.mp3", "Now playing: stopped", "Tab switch pane"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in %q", want, content)
		}
	}
	if strings.Count(content, "┌") < 5 {
		t.Errorf("expected five bordered panels: %q", content)
	}
	if strings.Contains(content, "00:00") || strings.Contains(content, "03:42") {
		t.Error("fabricated playback timing")
	}
	p.current = "01.mp3"
	if !strings.Contains(m.View().Content, "Now playing: 01.mp3") {
		t.Error("active track missing")
	}
	m.TracksFocused = true
	m.SelectedFolder = 1
	if !strings.Contains(m.View().Content, "other.mp3") || !strings.Contains(m.View().Content, "1 TRACKS") {
		t.Error("selection not reflected")
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
			if strings.Contains(view, "\x07") || strings.Contains(view, "\x1b[2J") {
				t.Fatalf("injected controls: %q", view)
			}
			view = stripStyle(view)
			for _, line := range strings.Split(view, "\n") {
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
	if strings.Index(lines[4], "TRACKS") < 0 || runewidth.StringWidth(strings.Split(lines[4], "TRACKS")[0]) != runewidth.StringWidth(strings.Split(lines[5], "song.mp3")[0]) {
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
				if strings.Contains(line, "FOLDERS") {
					headingRow = i
				}
				if strings.Contains(line, "TRACKS") && trackRow == -1 {
					trackRow = i
				}
			}
			if headingRow < 0 || trackRow < 0 || (headingRow == trackRow) != tt.sideBySide {
				t.Fatalf("unexpected pane headings at width %d: %q", tt.width, content)
			}
			if tt.sideBySide && strings.Index(lines[headingRow], "TRACKS") < strings.Index(lines[headingRow], "FOLDERS")+20 {
				t.Fatalf("tracks heading not in right column: %q", lines[headingRow])
			}
			if !strings.Contains(content, "other.mp3") {
				t.Fatalf("selected track missing: %q", content)
			}
		})
	}
}
func TestViewportPriorityAndWrapping(t *testing.T) {
	tracks := make([]string, 40)
	for i := range tracks {
		tracks[i] = "song.mp3"
	}
	m := New("root", library.Library{Playlists: []library.Playlist{{Name: "destination-long-name", Tracks: tracks}}}, nil, nil)
	m.Manager = &fakeManager{err: errors.New("actionable failure details")}
	m = press(m, "x")
	for _, size := range []tea.WindowSizeMsg{{Width: 1, Height: 8}, {Width: 24, Height: 9}, {Width: 80, Height: 10}} {
		next, _ := m.Update(size)
		m = next.(Model)
		content := m.View().Content
		lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
		if len(lines) > size.Height {
			t.Fatalf("viewport %v: %d lines", size, len(lines))
		}
		for _, line := range lines {
			if runewidth.StringWidth(stripStyle(line)) > size.Width {
				t.Fatalf("viewport %v: overflowing %q", size, line)
			}
		}
		if size.Width > 1 && (!strings.Contains(strings.ReplaceAll(content, "\n", ""), "destination-long-name") || !strings.Contains(strings.ReplaceAll(content, "\n", ""), "Windows Recycle Bin") || !strings.Contains(content, "Cancel")) {
			t.Fatalf("confirmation clipped at %v: %q", size, content)
		}
	}
	m = press(press(m, "down"), "enter")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 24, Height: 9})
	m = next.(Model)
	if !strings.Contains(strings.ReplaceAll(m.View().Content, "\n", ""), "actionable failure details") || !strings.Contains(m.View().Content, "Cancel") {
		t.Fatalf("feedback hidden: %q", m.View().Content)
	}
}

func TestTinyActionAndFocusedWindows(t *testing.T) {
	m := New("root", sample(), nil, nil)
	m.Manager = &fakeManager{err: errors.New("failed")}
	m.Width, m.Height = 24, 4
	m = press(m, "x")
	if !strings.Contains(m.View().Content, "Cancel") || !strings.Contains(m.View().Content, "resize to confirm") {
		t.Fatalf("tiny choice: %q", m.View().Content)
	}
	m = press(press(m, "down"), "enter")
	if m.DialogKind != "recycle" || strings.Contains(m.View().Content, "[Move]") {
		t.Fatalf("tiny window allowed Move: %q", m.View().Content)
	}
	playlists := make([]library.Playlist, 30)
	for i := range playlists {
		playlists[i] = library.Playlist{Name: fmt.Sprintf("folder-%02d", i), Tracks: []string{fmt.Sprintf("track-%02d", i)}}
	}
	m = New("root", library.Library{Playlists: playlists}, nil, nil)
	m.Width, m.Height, m.FolderIndex, m.SelectedFolder = 70, 12, 27, 27
	if !strings.Contains(m.View().Content, "folder-27") || !strings.Contains(m.View().Content, "track-27") {
		t.Fatalf("far folder: %q", m.View().Content)
	}
	tracks := make([]string, 30)
	for i := range tracks {
		tracks[i] = fmt.Sprintf("song-%02d", i)
	}
	m = New("root", library.Library{Playlists: []library.Playlist{{Name: "folder", Tracks: tracks}}}, nil, nil)
	m.Width, m.Height, m.TrackIndex, m.TracksFocused = 24, 12, 27, true
	if !strings.Contains(m.View().Content, "song-27") {
		t.Fatalf("narrow focused track: %q", m.View().Content)
	}
	if got := strings.ReplaceAll(wrapText("界abc", 1), "\n", ""); !strings.Contains(got, "abc") {
		t.Fatalf("wide rune stalled wrapping: %q", got)
	}
}

func TestBudgetedViewportRegressions(t *testing.T) {
	tracks := make([]string, 40)
	for i := range tracks {
		tracks[i] = fmt.Sprintf("song-%02d", i)
	}
	m := New(`D:\Music`, library.Library{Playlists: []library.Playlist{{Name: "folder", Tracks: tracks}}}, nil, &fakePlayer{current: "song-27"})
	m.Width, m.Height, m.TrackIndex, m.TracksFocused = 80, 24, 27, true
	for _, want := range []string{"Now playing: song-27", "Tab switch pane", "song-27"} {
		if !strings.Contains(m.View().Content, want) {
			t.Errorf("80x24 missing %q", want)
		}
	}
	m.Width, m.Height = 24, 10
	if !strings.Contains(m.View().Content, "song-27") {
		t.Errorf("24x10 selected track missing: %q", m.View().Content)
	}
	m = New(`D:\Music`, library.Library{}, errors.New("unavailable"), nil)
	m.Width, m.Height = 24, 6
	for _, want := range []string{`D:\Music`, "restore access"} {
		if !strings.Contains(strings.ReplaceAll(m.View().Content, "\n", ""), want) {
			t.Errorf("short root error missing %q: %q", want, m.View().Content)
		}
	}
	m = New("root", library.Library{Warnings: []error{errors.New("partial scan")}, Playlists: []library.Playlist{{Name: "folder", Tracks: tracks}}}, nil, nil)
	m.Width, m.Height = 80, 10
	if !strings.Contains(m.View().Content, "partial scan") {
		t.Errorf("warning missing: %q", m.View().Content)
	}
}

func TestCompactFocusedTrackAndRecycleFailure(t *testing.T) {
	tracks := make([]string, 40)
	for i := range tracks {
		tracks[i] = fmt.Sprintf("song-%02d", i)
	}
	m := New("root", library.Library{Playlists: []library.Playlist{{Name: "folder", Tracks: tracks}}}, nil, &fakePlayer{current: "song-00"})
	m.Width, m.Height, m.TrackIndex, m.TracksFocused = 24, 10, 39, true
	if !strings.Contains(m.View().Content, "› song-39") {
		t.Fatalf("focused last track hidden: %q", m.View().Content)
	}
	manager := &fakeManager{err: errors.New("disk full")}
	m = New("root", library.Library{Playlists: []library.Playlist{{Name: "very-long-recycle-folder-name"}}}, nil, nil)
	m.Manager, m.Width, m.Height = manager, 24, 4
	m = press(press(press(m, "x"), "down"), "enter")
	view := m.View().Content
	for _, want := range []string{"resize to confirm", "Cancel", "Esc cancels"} {
		if !strings.Contains(view, want) {
			t.Errorf("compact failure missing %q: %q", want, view)
		}
	}
	if len(strings.Split(stripStyle(view), "\n")) > 4 {
		t.Errorf("compact failure exceeds viewport: %q", view)
	}
	m = press(m, "enter")
	if len(manager.calls) != 0 || m.DialogKind != "recycle" {
		t.Fatalf("hidden move allowed: %+v %v", m, manager.calls)
	}
	m = press(m, "esc")
	if m.DialogKind != "" {
		t.Fatal("Esc did not cancel")
	}
}

func TestRecycleRejectsLossyOrClippedConfirmation(t *testing.T) {
	for _, tt := range []struct {
		name          string
		folder        string
		width, height int
		feedback      string
	}{
		{"wide rune at one cell", "界", 1, 80, ""},
		{"compact retry clips folder", "very-long-recycle-folder-name", 24, 4, "disk full"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manager := &fakeManager{err: errors.New("disk full")}
			m := New("root", library.Library{Playlists: []library.Playlist{{Name: tt.folder}}}, nil, nil)
			m.Manager, m.Width, m.Height = manager, tt.width, tt.height
			m = press(m, "x")
			m.Feedback = tt.feedback
			m = press(m, "down")
			if m.recycleConfirmationFits() || strings.Contains(m.View().Content, "[Move]") {
				t.Fatalf("lossy or clipped confirmation enabled Move: %q", m.View().Content)
			}
			m = press(m, "enter")
			if len(manager.calls) != 0 || m.DialogKind != "recycle" {
				t.Fatalf("Move executed without exact visible name: %v", manager.calls)
			}
			m.recycleMove = true
			next, _ := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			m = next.(Model)
			if m.recycleMove {
				t.Fatal("resize retained Move selection")
			}
		})
	}
}

func TestRecyclePromptNamesFolderAndContentsAndResizeClearsMove(t *testing.T) {
	m := New("root", library.Library{Playlists: []library.Playlist{{Name: "Jazz"}}}, nil, nil)
	m.Manager = &fakeManager{}
	m.Width, m.Height = 80, 24
	m = press(m, "x")
	const prompt = "Move folder Jazz and its contents to Windows Recycle Bin?"
	if !strings.Contains(m.View().Content, prompt) {
		t.Fatalf("missing complete confirmation: %q", m.View().Content)
	}
	m = press(m, "down")
	if !m.recycleMove || !strings.Contains(m.View().Content, "[Move]") {
		t.Fatalf("Move not selected by key: %q", m.View().Content)
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 10, Height: 2})
	m = next.(Model)
	if m.recycleMove || m.recycleConfirmationFits() || strings.Contains(m.View().Content, "[Move]") {
		t.Fatalf("resize exposed unsafe Move: %q", m.View().Content)
	}
}

func TestRecycleRequiresVisibleConfirmation(t *testing.T) {
	manager := &fakeManager{}
	name := "very-long-recycle-folder-name"
	m := New("root", library.Library{Playlists: []library.Playlist{{Name: name}}}, nil, nil)
	m.Manager = manager
	m.Scan = func(string) (library.Library, error) { return library.Library{}, nil }
	m.Width, m.Height = 24, 4
	m = press(press(m, "x"), "down")
	if !strings.Contains(m.View().Content, "resize to confirm") || strings.Contains(m.View().Content, "[Move]") {
		t.Fatalf("unsafe compact prompt: %q", m.View().Content)
	}
	m = press(m, "enter")
	if len(manager.calls) != 0 || m.DialogKind != "recycle" {
		t.Fatalf("hidden move executed: %+v %v", m, manager.calls)
	}
	m.Width, m.Height = 80, 24
	if m.recycleMove || !strings.Contains(strings.ReplaceAll(m.View().Content, "\n", ""), "Move folder "+name+" and its contents to Windows Recycle Bin?") {
		t.Fatalf("resize retained selection or lost name: %q", m.View().Content)
	}
	m = press(m, "enter")
	if len(manager.calls) != 0 || m.DialogKind != "" {
		t.Fatalf("resize did not default to cancel: %+v", m)
	}
	m = press(press(press(m, "x"), "down"), "enter")
	if strings.Join(manager.calls, ",") != "recycle:"+name {
		t.Fatalf("explicit move failed: %v", manager.calls)
	}
	m.Width, m.Height = 24, 4
	m = press(press(m, "x"), "esc")
	if m.DialogKind != "" {
		t.Fatal("Esc failed")
	}
}

func TestCompactFocusDistinctFromPlaying(t *testing.T) {
	tracks := make([]string, 40)
	for i := range tracks {
		tracks[i] = fmt.Sprintf("song-%02d", i)
	}
	m := New("root", library.Library{Playlists: []library.Playlist{{Name: "folder", Tracks: tracks}}}, nil, &fakePlayer{current: "song-00"})
	m.Width, m.Height, m.TrackIndex, m.TracksFocused = 24, 4, 39, true
	view := m.View().Content
	if !strings.Contains(view, "› song-39") || strings.Contains(view, "Now playing: song-39") {
		t.Fatalf("focus confused with playback: %q", view)
	}
}

func TestCompactViewportShowsFocusPlaybackAndHelp(t *testing.T) {
	tracks := make([]string, 40)
	for i := range tracks {
		tracks[i] = fmt.Sprintf("song-%02d", i)
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 12}, {Width: 24, Height: 8}} {
		t.Run(fmt.Sprintf("%dx%d", size.Width, size.Height), func(t *testing.T) {
			m := New("root", library.Library{Playlists: []library.Playlist{{Name: "folder", Tracks: tracks}}}, nil, &fakePlayer{current: "song-00"})
			m.Width, m.Height, m.TrackIndex, m.TracksFocused = size.Width, size.Height, 39, true
			lines := strings.Split(stripStyle(m.View().Content), "\n")
			if len(lines) > size.Height {
				t.Fatalf("%d visible lines exceed %d", len(lines), size.Height)
			}
			visible := strings.Join(lines, "\n")
			for _, want := range []string{"› song-39", "Now playing: song-00", "Tab switch pane"} {
				if !strings.Contains(visible, want) {
					t.Errorf("visible viewport missing %q: %q", want, visible)
				}
			}
		})
	}
}

func TestRecycleRetryFiveRowsRequiresVisibleFeedback(t *testing.T) {
	manager := &fakeManager{err: errors.New("disk full")}
	m := New("root", library.Library{Playlists: []library.Playlist{{Name: "Jazz"}}}, nil, nil)
	m.Manager, m.Width, m.Height = manager, 24, 5
	m = press(m, "x")
	m.Feedback = "disk full"
	m = press(m, "down")
	visible := strings.Join(strings.Split(stripStyle(m.View().Content), "\n")[:5], "\n")
	if strings.Contains(visible, "[Move]") && !strings.Contains(visible, "Error: disk full") {
		t.Errorf("Move visible without actionable feedback: %q", visible)
	}
	m = press(m, "enter")
	if len(manager.calls) != 0 {
		t.Fatalf("retry moved without visible feedback: %v", manager.calls)
	}
}

func TestRoomyLibraryHeightAndHints(t *testing.T) {
	m := New("root", sample(), nil, nil)
	m.Width, m.Height = 100, 30
	view := stripStyle(m.View().Content)
	lines := strings.Split(view, "\n")
	if len(lines) != 30 {
		t.Fatalf("roomy viewport should fill 30 rows, got %d", len(lines))
	}
	if !strings.Contains(view, "c Create") || !strings.Contains(view, "r Rename") || !strings.Contains(view, "x Move to Recycle Bin") || !strings.Contains(view, "r Rename track") {
		t.Fatalf("missing local hints: %q", view)
	}
	if !strings.Contains(view, "Now playing: stopped") || !strings.Contains(view, "Tab switch pane") {
		t.Fatalf("status/footer hidden: %q", view)
	}
	m.Height = 12
	compact := stripStyle(m.View().Content)
	if strings.Contains(compact, "Move to Recycle Bin") || len(strings.Split(compact, "\n")) > 12 {
		t.Fatalf("compact hints displaced content: %q", compact)
	}
	m.Height = 30
	if len(strings.Split(stripStyle(m.View().Content), "\n")) != 30 {
		t.Fatal("resize did not restore height")
	}
}

func stripStyle(s string) string {
	s = strings.ReplaceAll(s, "\x1b[48;2;13;10;18m", "")
	s = strings.ReplaceAll(s, "\x1b[38;2;190;185;195m", "")
	s = strings.ReplaceAll(s, "\x1b[38;2;245;84;189m", "")
	return strings.ReplaceAll(s, "\x1b[0m", "")
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
	m.Width = 180
	if !strings.Contains(m.View().Content, " c create ") || !strings.Contains(m.View().Content, " x recycle ") {
		t.Fatal("incorrect controls advertised")
	}
}
