# Local music TUI — product requirements

A Windows-first, keyboard-driven terminal player for local MP3 collections. This is a product contract, not an implementation plan. The approved magenta concept in `desing/music-player-magenta.op` and `desing/magenta-preview/` illustrates the two-pane library and confirmation state; behavior below takes precedence over visual details.

## Numbered requirements

1. **Launch and root.** A portable `music.exe` starts as `music` in Windows CMD and PowerShell when its directory is on `PATH`; no installer is mandatory. For v1.0 the library root is fixed at `D:\Music`; it already exists for the target user. If it is missing or inaccessible, show an actionable error naming `D:\Music` and do not silently create it or select another directory. A user-selectable root is deferred to a later release.
2. **Library.** Immediate subfolders of the root are playlists. List only MP3 files directly inside each folder; do not traverse nested folders. Show folders and tracks in separate panes with visible focus and selected folder.
3. **Playback.** Enter on a highlighted MP3 starts it and identifies it in the now-playing area. Space only toggles pause/resume; `s` stops playback and releases the active MP3 file. Browsing another folder does not interrupt playback; `n`/`p` remain bound to the active playback folder until Enter starts a track in another folder. Tracks are ordered alphabetically by filename (case-insensitive), not by numeric value or MP3 metadata; names such as `01.mp3`, `02.mp3`, and `10.mp3` appear in that order. The tracks pane and `n`/`p` use the same order. At the first/last track these controls do not wrap or cross folders. When a track finishes naturally, start the next track in the active playback folder in that order; after the last track, stop instead of wrapping or switching folders. Empty folders do not start playback.
4. **Management.** In the folders pane, `c` creates an immediate child of the root, `r` renames the selected folder, and `x` opens a confirmation before moving that folder and all its contents to the Windows Recycle Bin. In the tracks pane, `r` renames the selected MP3; there is no song deletion. Rename/create input rejects empty, Windows-invalid, and duplicate names with visible feedback and no changes. Enter validates and applies a valid name; Esc cancels without changes. Track rename edits only the file base name and always preserves the `.mp3` extension. The active track cannot be renamed while playing or paused; after `s` stops playback and releases the file, it can be renamed without quitting. A different selected track may be renamed while another track plays. Folder deletion defaults to Cancel and requires explicit Move confirmation; the root cannot be removed. Recycling failure leaves the folder intact with an error; never fall back to permanent deletion. Other filesystem failures do not claim success.
5. **Interaction.** Tab switches pane focus; Up/Down move the highlight within the focused pane and its bounds. Enter selects a folder in the folders pane or plays an MP3 in the tracks pane. Dialogs capture relevant keys; Esc cancels without changes. `q` exits from the main view. Controls remain discoverable in the UI.
6. **States and errors.** Show an understandable empty state for no playlist folders or no MP3 files in the selected folder. A missing/inaccessible `D:\Music` is reported with its path and guidance to create or restore access to that directory, without claiming the library loaded; no automatic directory creation or root chooser is offered in v1.0. For partially unreadable folders, keep accessible songs visible with a warning identifying unreadable entries. Playback failure is reported without falsely showing that track as playing. Keep the interface responsive where possible.

## Keyboard map

| Key | Main view | Dialog |
| --- | --- | --- |
| `Tab` | Switch folder/track focus | No pane change |
| `↑` / `↓` | Move highlight in focused pane | Choose confirmation action where applicable |
| `Enter` | Select folder / play highlighted MP3 | Validate create/rename input or activate confirmation action |
| `c` | Create folder, folders pane only | Text input where applicable |
| `r` | Rename selected folder or MP3 in focused pane | Text input where applicable |
| `x` | Open Recycle Bin confirmation, folders pane only | No repeat delete |
| `Space` | Pause/resume active track only; does not release its file | No playback change |
| `s` | Stop playback and release the active MP3 file | No playback change |
| `n` / `p` | Next / previous in active playback folder | No playback change |
| `q` | Quit main view | No quit; dismiss or complete dialog first |
| `Esc` | No change | Cancel without changes |

## User stories and observable acceptance criteria

| Story | Acceptance criteria |
| --- | --- |
| **US1 — Launch and browse.** As a Windows user, I want to open my local library from the terminal. | With portable `music.exe` on `PATH`, `music` opens the TUI in both CMD and PowerShell without requiring an installer. With default `D:\Music` containing `Rock/Intro.mp3`, `Rock/Notes.txt`, and `Rock/Live/Song.mp3`, only `Rock` appears as a playlist and only `Intro.mp3` appears in its tracks. Tab/arrows visibly change focus/highlight; Enter on `Rock` displays its tracks. |
| **US2 — Listen.** As a listener, I want playback independent from browsing. | Enter on a track starts playback and updates now-playing; Space pauses/resumes without releasing the active MP3; `s` stops playback, clears the active playback state, and releases its file so it can be renamed without quitting. While that track plays, selecting another folder leaves playback unchanged and `n`/`p` stay within the original playback folder. Enter on a track in the new folder changes the active playback folder; subsequent `n`/`p` use it. The tracks pane and `n`/`p` follow case-insensitive alphabetical filename order (`01`, `02`, `10` with zero padding), not numeric or MP3-metadata order. At either edge controls do not wrap or cross folders; with no active track they do not invent one. Natural completion starts the next track in the active playback folder, even while browsing another folder; completion of the last track stops playback without wrapping. |
| **US3 — Manage folders and tracks.** As a listener, I want to organize my library without deleting individual songs. | `c` creates a folder; `r` in the folders pane renames the selected folder; `r` in the tracks pane edits only the selected MP3's base name; the resulting filename always retains its `.mp3` extension. While the selected track is active, `r` cannot rename it during playback or pause and gives visible feedback without changing the file. After `s`, the formerly active track can be renamed without quitting. A non-active selected track can be renamed while another track plays. Lists reflect successful changes. Enter on valid input applies it; Esc leaves names unchanged. Empty, Windows-invalid or duplicate names show an error and do not alter files/folders; filesystem failures likewise show an error without claiming success. No song delete action appears. |
| **US4 — Safely remove a playlist.** As a listener, I want a reversible removal path. | `x` on a folder opens a dialog naming it and explaining that its contents move to the Windows Recycle Bin. Cancel is selected by default; Esc or Enter on Cancel keeps it intact. Only explicitly selecting and confirming Move sends the folder and contents to the Recycle Bin. The root is never eligible. On failure or unsupported recycling, the original folder remains and an error is shown; permanent deletion is never offered as fallback. |
| **US5 — Recover from empty or broken input.** As a listener, I want clear feedback instead of a broken screen. | An empty root shows no playlists; an empty playlist shows no playable tracks. If `D:\Music` is missing or inaccessible, show a clear error naming that path and how to create it or restore access; do not create or change the root automatically, and do not imply a loaded library. If some folder entries cannot be read, accessible MP3s remain listed and a warning identifies the unreadable entries. Failed playback reports an error without showing that track as successfully playing; navigation/cancellation/quit remain responsive where possible. |

## Boundaries and non-goals

- No root-folder chooser/configuration in v1.0, recursive browsing, nested playlist discovery, song-level deletion, user-defined playlists, non-MP3 playback, streaming, or cloud library.
- No mandated Go/TUI/audio library: Go, Bubble Tea, Oto, and go-mp3 are candidates, not commitments.
- No software is implemented by this document. Exact colors, dimensions, sample filenames, counts, and progress display in the mockup are illustrative.

## Open decisions (not approved behavior)

- **OD4 — Runtime compatibility:** exact supported Windows versions and audio runtime/prerequisites are unverified; no installer or particular PATH setup mechanism is mandated.
