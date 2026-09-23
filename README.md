# Music CLI — playback compatibility probe (I1)

This is **not yet the music player TUI**. It is a bounded Windows-first MP3 playback probe; browsing, library scanning, file management, and the TUI are deferred. No music file is modified.

```text
go test ./...
go build ./...
go run ./cmd/music "C:\path\to\01.mp3" "C:\path\to\02.mp3"
```

Use your own licensed local MP3 files from the **same folder** and an available audio device. The first argument starts playback; the supplied filenames are sorted case-insensitively for next/previous and natural completion. Type `n`, `p`, `s`, or `q` followed by Enter; press Enter on an empty line to pause/resume. `s` releases the file; `q` exits. No wrap at queue edges. This probe has not been audibly verified without an actual MP3 and output device.

The device uses Oto v3.5.1 and go-mp3 v0.3.4 (unmaintained upstream). Oto has one process-wide sample rate; a subsequent file with a different rate fails explicitly rather than playing at the wrong speed. Restart the probe to play that file. Decoder/runtime/device compatibility remains subject to manual testing on target Windows versions. A read or device failure is not an audible success; asynchronous failures print to stderr without waiting for another command, including when the probe is stopped or quit.
