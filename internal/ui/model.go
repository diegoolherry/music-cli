// Package ui provides a keyboard-driven library browser without owning playback queues.
package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

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

// Manager applies validated library mutations; playback remains independent.
type Manager interface {
	CreatePlaylist(string) error
	RenamePlaylist(string, string) error
	RecyclePlaylist(string) error
	RenameTrack(string, string, string) error
}

type Model struct {
	Manager        Manager
	Scan           func(string) (library.Library, error)
	DialogKind     string
	DialogInput    string
	dialogFolder   string
	dialogTrack    string
	recycleMove    bool
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
	Height         int
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
		m.Height = size.Height
		if m.DialogKind == "recycle" {
			m.recycleMove = false // A resize requires a fresh, visible selection.
		}
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
	if m.DialogKind == "recycle" {
		if !m.recycleConfirmationFits() {
			m.recycleMove = false
			if key.String() == "esc" {
				m.DialogKind = ""
				m.Feedback = ""
			}
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.DialogKind = ""
			m.Feedback = ""
		case "up", "down":
			m.recycleMove = !m.recycleMove
		case "enter":
			if !m.recycleMove {
				m.DialogKind = ""
				m.Feedback = ""
				break
			}
			if m.Manager == nil || m.RootError != nil || m.TracksFocused || m.dialogFolder == "" {
				m.Feedback = "playlist recycling unavailable"
				break
			}
			if err := m.Manager.RecyclePlaylist(m.dialogFolder); err != nil {
				m.report(err)
				m.recycleMove = false
				break
			}
			m.DialogKind = ""
			if m.Scan == nil {
				m.Feedback = "library scan unavailable; refresh before further changes"
				m.RootError = fmt.Errorf("library scan unavailable")
				break
			}
			snapshot, err := m.Scan(m.Root)
			if err != nil {
				m.RootError = err
				m.report(err)
				break
			}
			m.Library = snapshot
			m.RootError = nil
			m.FolderIndex, m.SelectedFolder, m.TrackIndex = 0, 0, 0
			m.Feedback = ""
		}
		return m, nil
	}
	if m.DialogKind != "" {
		switch key.String() {
		case "esc":
			m.DialogKind = ""
			m.DialogInput = ""
			m.Feedback = ""
		case "backspace":
			if len(m.DialogInput) > 0 {
				r := []rune(m.DialogInput)
				m.DialogInput = string(r[:len(r)-1])
			}
		case "enter":
			if m.Manager == nil {
				m.Feedback = "library manager unavailable"
				break
			}
			var err error
			switch m.DialogKind {
			case "create":
				err = m.Manager.CreatePlaylist(m.DialogInput)
			case "folder":
				err = m.Manager.RenamePlaylist(m.dialogFolder, m.DialogInput)
			case "track":
				err = m.Manager.RenameTrack(m.dialogFolder, m.dialogTrack, m.DialogInput)
			}
			if err != nil {
				m.report(err)
				break
			}
			m.DialogKind = ""
			m.DialogInput = ""
			if m.Scan == nil {
				m.Feedback = "library scan unavailable"
				break
			}
			snapshot, scanErr := m.Scan(m.Root)
			m.RootError = scanErr
			if scanErr != nil {
				m.report(scanErr)
				break
			}
			m.Library = snapshot
			// A changed sort order invalidates old numeric selections.
			m.FolderIndex, m.SelectedFolder, m.TrackIndex = 0, 0, 0
			m.Feedback = ""
		default:
			if key.Mod == 0 && key.Text != "" {
				for _, r := range key.Text {
					if r >= 32 && r != 127 {
						m.DialogInput += string(r)
					}
				}
			}
		}
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
	case "c":
		if !m.TracksFocused && m.RootError == nil && m.Manager != nil {
			m.DialogKind = "create"
			m.DialogInput = ""
		}
	case "r":
		if m.RootError == nil && m.Manager != nil && len(m.Library.Playlists) > 0 {
			if m.TracksFocused {
				if tracks := m.tracks(); len(tracks) > 0 {
					m.dialogFolder = m.Library.Playlists[m.SelectedFolder].Name
					m.dialogTrack = tracks[m.TrackIndex]
					m.DialogKind = "track"
					m.DialogInput = strings.TrimSuffix(m.dialogTrack, filepath.Ext(m.dialogTrack))
				}
			} else {
				m.dialogFolder = m.Library.Playlists[m.FolderIndex].Name
				m.DialogKind = "folder"
				m.DialogInput = m.dialogFolder
			}
		}
	case "x":
		if !m.TracksFocused && m.RootError == nil && m.Manager != nil && len(m.Library.Playlists) > 0 {
			m.FolderIndex = clamp(m.FolderIndex, len(m.Library.Playlists))
			m.dialogFolder = m.Library.Playlists[m.FolderIndex].Name
			m.recycleMove = false
			m.DialogKind = "recycle"
			m.Feedback = ""
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

// recycleConfirmationFits requires the entire destination and both actions to be visible.
func (m Model) recycleConfirmationFits() bool {
	width := m.Width
	if width <= 0 {
		width = 80
	}
	if m.Height <= 0 {
		return true
	}
	promptText := "Move folder " + safeText(m.dialogFolder) + " and its contents to Windows Recycle Bin?"
	for _, r := range promptText {
		if runewidth.RuneWidth(r) > width {
			return false // wrapText substitutes an unrenderable wide rune with '?'.
		}
	}
	prompt := wrapText(promptText, width)
	choice := wrapText("Cancel   [Move] ↑/↓ Enter Esc", width)
	needed := len(strings.Split(prompt, "\n")) + len(strings.Split(choice, "\n"))
	if m.Feedback != "" {
		needed += len(strings.Split(wrapText("Error: "+safeText(m.Feedback), width), "\n"))
	}
	if needed > m.Height {
		return false
	}
	// The compact retry view replaces the wrapped prompt with two clipped lines.
	if m.Feedback != "" && m.Height <= 4 {
		if m.Height < 4 {
			return false // Compact retry needs prompt, action, and feedback on four visible lines.
		}
		return clipText("Move folder "+safeText(m.dialogFolder), width) == "Move folder "+safeText(m.dialogFolder) &&
			clipText("and its contents to Windows Recycle Bin?", width) == "and its contents to Windows Recycle Bin?" &&
			clipText("Cancel [Move] ↑/↓ Enter", width) == "Cancel [Move] ↑/↓ Enter"
	}
	return true
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
	panel := func(lines []string, w int) []string {
		if w < 2 {
			return []string{"│"}
		}
		inner := w - 2
		accent := "\x1b[38;2;245;84;189m"
		neutral := "\x1b[38;2;190;185;195m"
		out := []string{accent + "┌" + strings.Repeat("─", inner) + "┐" + neutral}
		for _, line := range lines {
			text := clipText(line, inner)
			out = append(out, accent+"│"+neutral+text+strings.Repeat(" ", inner-runewidth.StringWidth(text))+accent+"│"+neutral)
		}
		return append(out, accent+"└"+strings.Repeat("─", inner)+"┘"+neutral)
	}
	writePanel := func(lines []string) {
		for _, line := range panel(lines, width) {
			b.WriteString(line + "\n")
		}
	}
	// Reserve space for actionable messages before rendering the scrollable library.
	reserved := 7 // Header (3), playback (3), footer (3), library borders (2).
	reserved += 4
	if m.Height > 0 && m.Height <= 12 {
		reserved = 4 // Compact mode can discard decoration to expose focused rows.
	}
	if m.DialogKind != "" {
		reserved += 4
	}
	if m.Feedback != "" {
		reserved += 2
	}
	reserved += len(m.Library.Warnings)
	roomy := m.Height >= 20 && width >= 48 && m.DialogKind == "" && m.Feedback == "" && m.RootError == nil
	if roomy {
		reserved = 14 + len(m.Library.Warnings) // Three separating rows in addition to the four panel frames.
	}
	libraryRows := 1 << 20
	if m.Height > 0 {
		libraryRows = m.Height - reserved
		if libraryRows < 0 {
			libraryRows = 0
		}
	}
	writePanel([]string{" MUSIC  LOCAL LIBRARY  " + safeText(m.Root)})
	if roomy {
		b.WriteByte('\n')
	}
	if m.RootError != nil {
		fmt.Fprintln(&b, clipText(fmt.Sprintf("Library %s unavailable: create or restore access to this directory. %v", safeText(m.Root), safeText(m.RootError.Error())), width))
	} else {
		folderFocus, trackFocus := " ", " "
		if m.TracksFocused {
			trackFocus = "▶"
		} else {
			folderFocus = "▶"
		}
		folders := []string{folderFocus + " FOLDERS / PLAYLISTS", fmt.Sprintf("  %d PLAYLISTS", len(m.Library.Playlists))}
		tracks := []string{trackFocus + fmt.Sprintf(" TRACKS · %d TRACKS", len(m.tracks()))}
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
		if roomy {
			folders = append(folders, "", "c Create   r Rename", "x Move to Recycle Bin")
			tracks = append(tracks, "", "r Rename track")
		}
		if width < 48 {
			if libraryRows > 0 {
				folderRows := libraryRows
				if m.TracksFocused && folderRows > 1 {
					folderRows = 1
				}
				folders = windowRows(folders, folderRows, m.FolderIndex+2)
				writePanel(folders)
			}
			remaining := libraryRows - len(folders) - 2
			if m.TracksFocused && remaining < 1 && m.Height >= 8 {
				remaining = 1
			}
			if remaining > 0 {
				tracks = windowRows(tracks, remaining, m.TrackIndex+1)
				writePanel(tracks)
			}
		} else {
			leftWidth := width / 3
			if leftWidth < 20 {
				leftWidth = 20
			}
			rightWidth := width - leftWidth
			rows := len(folders)
			if len(tracks) > rows {
				rows = len(tracks)
			}
			if roomy {
				rows = libraryRows
			}
			if rows > libraryRows {
				rows = libraryRows
			}
			folders = windowRows(folders, rows, m.FolderIndex+2)
			tracks = windowRows(tracks, rows, m.TrackIndex+1)
			for len(folders) < rows {
				folders = append(folders, "")
			}
			for len(tracks) < rows {
				tracks = append(tracks, "")
			}
			left, right := panel(folders, leftWidth), panel(tracks, rightWidth)
			for i := range left {
				b.WriteString(left[i] + right[i] + "\n")
			}
		}
		for _, warning := range m.Library.Warnings {
			fmt.Fprintln(&b, clipText("Warning: "+safeText(fmt.Sprint(warning)), width))
		}
	}
	if roomy {
		b.WriteByte('\n')
	}
	if m.Player != nil && m.Player.Current() != "" {
		writePanel([]string{"Now playing: " + safeText(m.Player.Current())})
	} else {
		writePanel([]string{"Now playing: stopped"})
	}
	if m.DialogKind == "recycle" {
		fmt.Fprintln(&b, wrapText("Move folder "+safeText(m.dialogFolder)+" and its contents to Windows Recycle Bin?", width))
		choices := "[Cancel]   Move"
		if !m.recycleConfirmationFits() {
			choices = "[Cancel] resize to confirm · Esc"
		} else if m.recycleMove {
			choices = "Cancel   [Move]"
		}
		fmt.Fprintln(&b, wrapText(choices+" ↑/↓ Enter Esc", width))
	} else if m.DialogKind != "" {
		fmt.Fprintln(&b, wrapText("Dialog "+m.DialogKind+": "+safeText(m.DialogInput)+" (Enter apply · Esc cancel)", width))
	}
	if m.Feedback != "" {
		fmt.Fprintln(&b, wrapText("Error: "+safeText(m.Feedback), width))
	}
	if roomy {
		b.WriteByte('\n')
	}
	writePanel([]string{"Tab switch pane · ↑/↓ navigate · Enter select/play · c create · r rename · x recycle · Space pause/resume · n/p next/previous · s stop · q quit"})
	if m.Height > 0 {
		lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
		if m.Height <= 12 && m.RootError == nil && m.DialogKind == "" && m.Feedback == "" {
			// Preserve the selected row even when the full panel stack cannot fit.
			focus := "› "
			if m.TracksFocused && len(m.tracks()) > 0 {
				focus += safeText(m.tracks()[clamp(m.TrackIndex, len(m.tracks()))])
			} else if len(m.Library.Playlists) > 0 {
				focus = "›"
				focus += "* " + safeText(m.Library.Playlists[clamp(m.FolderIndex, len(m.Library.Playlists))].Name)
			}
			visible := false
			for _, line := range lines[:min(len(lines), m.Height)] {
				if strings.Contains(line, clipText(focus, width-2)) {
					visible = true
				}
			}
			if !visible && focus != "› " {
				lines = append([]string{clipText(focus, width)}, lines...)
			}
		}
		if m.Height <= 12 && m.DialogKind == "" && m.Feedback == "" && m.RootError == nil && m.Height >= 3 {
			status := "Now playing: stopped"
			if m.Player != nil && m.Player.Current() != "" {
				status = "Now playing: " + safeText(m.Player.Current())
			}
			body := lines[:min(len(lines), m.Height-2)]
			focus := ""
			if m.TracksFocused && len(m.tracks()) > 0 {
				focus = "› " + safeText(m.tracks()[clamp(m.TrackIndex, len(m.tracks()))])
			} else if len(m.Library.Playlists) > 0 {
				focus = "›* " + safeText(m.Library.Playlists[clamp(m.FolderIndex, len(m.Library.Playlists))].Name)
			}
			if focus != "" && !strings.Contains(strings.Join(body, "\n"), clipText(focus, width-2)) {
				if len(body) == m.Height-2 {
					body = body[:len(body)-1]
				}
				body = append(body, clipText(focus, width))
			}
			if len(m.Library.Warnings) > 0 {
				warning := clipText("Warning: "+safeText(fmt.Sprint(m.Library.Warnings[0])), width)
				if !strings.Contains(strings.Join(body, "\n"), warning) {
					if len(body) == m.Height-2 {
						body = body[:len(body)-1]
					}
					body = append(body, warning)
				}
			}
			lines = append(body, clipText(status, width), clipText("Tab switch pane · ↑/↓ navigate · Enter select/play", width))
		}
		if m.DialogKind == "recycle" && m.Feedback != "" && m.Height <= 4 && m.recycleConfirmationFits() {
			choice := "[Cancel] Move ↑/↓ Enter"
			if m.recycleMove {
				choice = "Cancel [Move] ↑/↓ Enter"
			}
			lines = []string{
				clipText("Move folder "+safeText(m.dialogFolder), width),
				clipText("and its contents to Windows Recycle Bin?", width),
				clipText(choice, width),
				clipText("Error: "+safeText(m.Feedback), width),
			}
		}

		if len(lines) > m.Height {
			// Fit priority content before optional library rows; never truncate a dialog
			// by taking an arbitrary prefix of the fully assembled display.
			if m.DialogKind == "" && m.Feedback == "" && m.RootError == nil {
				var priority []string
				for _, line := range lines {
					if strings.Contains(line, "Warning:") {
						priority = append(priority, line)
					}
				}
				if len(priority) > 0 && len(priority) < m.Height {
					lines = append(priority, lines[:m.Height-len(priority)]...)
				}
			}
			// On tiny screens, render actions first, then status, then decoration.
			if m.DialogKind != "" || m.Feedback != "" || m.RootError != nil {
				var priority []string
				if m.DialogKind == "recycle" {
					priority = append(priority, strings.Split(wrapText("Move folder "+safeText(m.dialogFolder)+" and its contents to Windows Recycle Bin?", width), "\n")...)
					choice := "[Cancel]   Move"
					if !m.recycleConfirmationFits() {
						priority = []string{clipText("resize to confirm", width), clipText("[Cancel] Esc cancels", width)}
					} else if m.recycleMove {
						choice = "Cancel   [Move]"
					}
					if m.recycleConfirmationFits() {
						priority = append(priority, strings.Split(wrapText(choice+" ↑/↓ Enter Esc", width), "\n")...)
					}
				} else if m.DialogKind != "" {
					priority = append(priority, strings.Split(wrapText("Dialog "+m.DialogKind+": "+safeText(m.DialogInput)+" (Enter apply · Esc cancel)", width), "\n")...)
				}
				if m.Feedback != "" {
					priority = append(priority, strings.Split(wrapText("Error: "+safeText(m.Feedback), width), "\n")...)
				}
				if m.RootError != nil {
					priority = append(priority, strings.Split(wrapText("Library "+safeText(m.Root)+" unavailable: restore access to this directory. "+safeText(m.RootError.Error()), width), "\n")...)
				}
				if len(priority) >= m.Height {
					lines = priority[:m.Height]
				} else {
					lines = append(priority, lines[:m.Height-len(priority)]...)
				}
			} else if len(lines) > m.Height {
				lines = lines[:m.Height]
			}
		}
		b.Reset()
		b.WriteString(strings.Join(lines, "\n"))
	}
	// Style only trusted frame output; untrusted values are sanitized before insertion.
	v := tea.NewView("\x1b[48;2;13;10;18m\x1b[38;2;190;185;195m" + b.String() + "\x1b[0m")
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

// windowRows keeps headings when possible and scrolls the selected item into view.
func windowRows(lines []string, limit, focus int) []string {
	if limit <= 0 {
		return nil
	}
	if len(lines) <= limit {
		return lines
	}
	heading := 0
	if focus > 0 && limit > 1 {
		heading = focus
		if heading > 2 {
			heading = 2
		}
		if heading > limit-1 {
			heading = limit - 1
		}
	}
	start := focus - heading
	if start < 0 {
		start = 0
	}
	if start+limit > len(lines) {
		start = len(lines) - limit
	}
	if start == 0 {
		return lines[:limit]
	}
	if heading > 0 {
		return append(append([]string{}, lines[:heading]...), lines[start+heading:start+limit]...)
	}
	return lines[start : start+limit]
}

func wrapText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	var lines []string
	for s != "" {
		part := clipText(s, width)
		if part == "" {
			// A wide rune cannot fit in one cell; replace it and keep later text.
			_, size := utf8.DecodeRuneInString(s)
			lines = append(lines, "?")
			s = s[size:]
			continue
		}
		lines = append(lines, part)
		s = s[len(part):]
	}
	return strings.Join(lines, "\n")
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
