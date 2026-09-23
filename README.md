# Music CLI — Windows music library (I3)

Run `music` to open the two-pane terminal library at the fixed `D:\Music` root. Immediate subfolders are playlists; only MP3 files directly inside each playlist appear. A loose MP3 directly under `D:\Music` is **not** a playlist track. The program never moves or changes music files. A missing or inaccessible root displays guidance; there is no root chooser.

Build with `go build -o music.exe ./cmd/music`, then put the executable's directory on PATH to invoke `music` from CMD or PowerShell. This terminal/audio runtime has not yet been manually verified in either shell. Use Tab to switch panes, arrows to navigate, Enter to select a folder or play a track, Space to pause/resume, `n`/`p` for the active folder's ordered queue, `s` to stop/release, and `q` to quit/release. Folder creation, rename, and recycle controls are not yet implemented.

To listen to an MP3 directly under `D:\Music` without moving it into a playlist, run `music --probe "D:\Music\your-song.mp3"` (or `go run ./cmd/music --probe "D:\Music\your-song.mp3"`). Supply multiple MP3 paths from the same folder for next/previous. The probe reads files without modifying them: type `n`, `p`, `s`, or `q` followed by Enter; an empty line pauses/resumes. Playback has not been audibly confirmed on the target machine.

Oto v3.5.1 and go-mp3 v0.3.4 provide audio; go-mp3 is unmaintained upstream. Oto's process-wide sample rate means a later MP3 with another rate fails explicitly; restart the program for that track. Decoder, device, and Windows runtime compatibility require manual confirmation.
