// Package ui provides a keyboard-driven library browser without owning playback queues.
package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"

	tea "charm.land/bubbletea/v2"
	"github.com/diegoolherry/music-cli/internal/library"
)

// Player is implemented by playback.Engine; its queue remains independent of browsing.
type Player interface {
	Play(string, []string, string) error
	Current() string
	Next() error
	Previous() error
	Pause() error
	Stop() error
	Errors() <-chan error
}

type playbackNotice struct{ err error }

func awaitPlayback(p Player) tea.Cmd {
	return func() tea.Msg {
		select {
		case err := <-p.Errors():
			return playbackNotice{err: err}
		case <-time.After(150 * time.Millisecond):
			return playbackNotice{}
		}
	}
}

type Model struct {
	Root           string
	Library        library.Library
	RootError      error
	Player         Player
	FolderIndex    int
	TrackIndex     int
	SelectedFolder int
	TracksFocused  bool
	Feedback       string
	QuitError      error
	Width          int
}

func New(root string, snapshot library.Library, rootErr error, player Player) Model {
	return Model{Root: root, Library: snapshot, RootError: rootErr, Player: player}
}
func (m Model) Init() tea.Cmd {
	if m.Player != nil {
		return awaitPlayback(m.Player)
	}
	return nil
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.Width = size.Width
		return m, nil
	}
	if notice, ok := msg.(playbackNotice); ok {
		if notice.err != nil {
			m.Feedback = notice.err.Error()
		}
		if m.Player != nil {
			return m, awaitPlayback(m.Player)
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "q", "ctrl+c":
		if m.Player != nil {
			m.QuitError = m.Player.Stop()
			m.report(m.QuitError)
			select {
			case err := <-m.Player.Errors():
				if err != nil {
					m.Feedback = err.Error()
					m.QuitError = err
				}
			default:
			}
		}
		return m, tea.Quit
	case "tab":
		m.TracksFocused = !m.TracksFocused
	case "up", "down":
		delta := 1
		if key.String() == "up" {
			delta = -1
		}
		if m.TracksFocused {
			m.TrackIndex = clamp(m.TrackIndex+delta, len(m.tracks()))
		} else {
			m.FolderIndex = clamp(m.FolderIndex+delta, len(m.Library.Playlists))
		}
	case "enter":
		if m.RootError != nil {
			break
		}
		if !m.TracksFocused {
			if len(m.Library.Playlists) > 0 {
				m.SelectedFolder = m.FolderIndex
				m.TrackIndex = 0
			}
		} else if tracks := m.tracks(); len(tracks) > 0 && m.Player != nil {
			folder := m.Library.Playlists[m.SelectedFolder]
			m.report(m.Player.Play(filepath.Join(m.Root, folder.Name), tracks, tracks[m.TrackIndex]))
		}
	case "n":
		if m.Player != nil {
			m.report(m.Player.Next())
		}
	case "p":
		if m.Player != nil {
			m.report(m.Player.Previous())
		}
	case "space":
		if m.Player != nil {
			m.report(m.Player.Pause())
		}
	case "s":
		if m.Player != nil {
			m.report(m.Player.Stop())
		}
	}
	return m, nil
}
func clamp(i, n int) int {
	if n == 0 || i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}
func (m Model) tracks() []string {
	if m.RootError != nil || len(m.Library.Playlists) == 0 {
		return nil
	}
	return m.Library.Playlists[clamp(m.SelectedFolder, len(m.Library.Playlists))].Tracks
}
func (m *Model) report(err error) {
	if err != nil {
		m.Feedback = err.Error()
	} else {
		m.Feedback = ""
	}
}
func (m Model) View() tea.View {
	var b strings.Builder
	width := m.Width
	if width <= 0 {
		width = 80
	}
	b.WriteString("\x1b[48;2;13;10;18m\x1b[38;2;245;84;189m" + clipText(" MUSIC LIBRARY", width) + "\x1b[0m\n")
	if m.RootError != nil {
		fmt.Fprintln(&b, clipText(fmt.Sprintf("Library %s unavailable: create or restore access to this directory. %v", safeText(m.Root), safeText(m.RootError.Error())), width))
	} else {
		folderFocus, trackFocus := " ", " "
		if m.TracksFocused {
			trackFocus = "▶"
		} else {
			folderFocus = "▶"
		}
		folders := []string{folderFocus + " Folders"}
		tracks := []string{trackFocus + " Tracks"}
		if len(m.Library.Playlists) == 0 {
			folders = append(folders, "No playlist folders found.")
		}
		for i, p := range m.Library.Playlists {
			marker := " "
			if i == m.FolderIndex {
				marker = "›"
			}
			selected := " "
			if i == m.SelectedFolder {
				selected = "*"
			}
			folders = append(folders, marker+selected+" "+safeText(p.Name))
		}
		if len(m.Library.Playlists) > 0 && len(m.tracks()) == 0 {
			tracks = append(tracks, "No MP3 tracks in this folder.")
		}
		for i, name := range m.tracks() {
			marker := " "
			if i == m.TrackIndex {
				marker = "›"
			}
			tracks = append(tracks, marker+" "+safeText(name))
		}
		if width < 48 {
			for _, line := range folders {
				fmt.Fprintln(&b, clipText(line, width))
			}
			for _, line := range tracks {
				fmt.Fprintln(&b, clipText(line, width))
			}
		} else {
			leftWidth := width / 3
			if leftWidth < 20 {
				leftWidth = 20
			}
			rightWidth := width - leftWidth - 2
			rows := len(folders)
			if len(tracks) > rows {
				rows = len(tracks)
			}
			for i := 0; i < rows; i++ {
				left, right := "", ""
				if i < len(folders) {
					left = clipText(folders[i], leftWidth)
				}
				if i < len(tracks) {
					right = clipText(tracks[i], rightWidth)
				}
				b.WriteString(left + strings.Repeat(" ", leftWidth-runewidth.StringWidth(left)) + "  " + right + "\n")
			}
		}
		for _, warning := range m.Library.Warnings {
			fmt.Fprintln(&b, clipText("Warning: "+safeText(fmt.Sprint(warning)), width))
		}
	}
	if m.Player != nil && m.Player.Current() != "" {
		fmt.Fprintln(&b, clipText("Now playing: "+safeText(m.Player.Current()), width))
	} else {
		fmt.Fprintln(&b, clipText("Now playing: stopped", width))
	}
	if m.Feedback != "" {
		fmt.Fprintln(&b, clipText("Error: "+safeText(m.Feedback), width))
	}
	b.WriteString(clipText("Tab switch pane · ↑/↓ navigate · Enter select/play · Space pause/resume · n/p next/previous · s stop · q quit", width))
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// safeText prevents filenames and external errors from controlling terminal rendering.
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || (r >= 0x80 && r <= 0x9f) {
			return ' '
		}
		return r
	}, s)
}

func clipText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		cells := runewidth.RuneWidth(r)
		if cells > width {
			break
		}
		b.WriteRune(r)
		width -= cells
	}
	return b.String()
}
