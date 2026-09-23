# Local music player planning

Objective: Produce an editable TUI concept and a testable requirements document for a lightweight Windows-first local MP3 player, before application implementation.

## Scope and constraints

- Go is the proposed implementation language; Bubble Tea and Oto/go-mp3 are candidates, not verified implementation commitments.
- V1.0 uses existing `D:\Music` as fixed music root; if missing/inaccessible, show setup guidance without silently creating or choosing a different root. Root selection is deferred to a later release. Immediate subfolders represent playlists containing MP3 files directly inside.
- Two-pane keyboard-driven terminal view; `music` launches the application in Windows CMD or PowerShell when `music.exe` is installed on PATH.
- Folder actions: create, rename, and `x` to request deletion with confirmation; deletion of folders containing songs must use Windows Recycle Bin, not permanent deletion. `r` also renames a selected song in the tracks pane.
- Browsing another folder never interrupts the playing track; `n`/`p` stay bound to the active playback playlist until Enter starts a song in a different playlist. `s` stops playback and releases the file so it can be renamed; rename preserves `.mp3` and is blocked for the active track until stopped. Partially unreadable folders keep accessible content visible with a warning.
- No application source implementation, song deletion, configurable playlists, or recursive folder navigation in this planning feature.
- Technical artifacts use English; conversation remains Spanish.

## Tasks

- [x] T1: Create an editable OpenPencil TUI mockup at `desing/music-player.op`, export a viewable preview under `desing/`, and validate/read back the layout. Check: mockup shows both panes, playback and navigation controls, `x` delete affordance and confirmation state; preview opens.
- [x] T2: Write requirements, user stories, acceptance criteria, boundary cases, non-goals, and dependency/install assumptions to `desing/requirements.md`. Check: each story has observable criteria; read back file and ensure it agrees with the mockup and user decisions.
- [x] T3: Preserve the original mockup and create a darker magenta OpenPencil variant with PNG previews. Check: both frames refine and export, new palette is visually inspected, original files remain unchanged.
- [x] T4: Align `desing/requirements.md` and the approved magenta mockup with the accepted OD1–OD5 decisions and song rename in the tracks pane. Check: read back new criteria; OpenPencil refine/export both frames; no claim that queue ordering, auto-next, missing-root setup, or in-use rename policy was approved.
- [x] T5: Document approved safe MP3 rename and `s` stop key; add the stop hint to the magenta mockup. Check: requirements clearly distinguish stop from pause, blocked active/paused rename and preserved extension; both mockup frames refine/export, preview visually inspected.
- [x] T6: Close v1.0 root behavior in `desing/requirements.md`: fixed `D:\Music`, clear missing/inaccessible-root guidance, deferred root chooser. Check: criteria and non-goals agree; read back.
- [x] T7: Define case-insensitive alphabetical filename order and automatic next-track playback within the active folder. Check: numbered playback requirements and US2 agree; OD2 removed.

## Verification and delivery

- Design: OpenPencil refine/lint and export/readback if available.
- Requirements: inspect content for contradictions and confirm status of non-implemented behavior.
- TDD: not applicable to these planning artifacts; no application tests or runtime harness yet.
- Delivery strategy: ask-on-risk; expected authored lines below 400 (design binary/generated assets excluded).
- Work-unit commits: pending explicit authorization; do not commit without a user request.

## Progress

- T1 done: OpenPencil refine found zero fixes on both frames; exported both PNGs and visually inspected them. Uncommitted pending explicit authorization.
- T2 done: delegated writer ran `test -s desing/requirements.md` successfully; parent read back five user stories, criteria, keyboard map, non-goals and open decisions and confirmed agreement with mockup. No app tests (planning-only).
- T3 done: saved `desing/music-player-magenta.op`; OpenPencil refined both frames and exported PNGs in `desing/magenta-preview/`. Parent visually inspected both screens. Original `.op` and previews were preserved.
- T4 done: delegated writer verified nonempty requirements and zero OpenPencil refine fixes on both frames; parent read back criteria and visually inspected both updated PNGs (`D:/Music`, contextual `r Rename track`). Original mockup untouched; no app runtime verification.
- T5 done: delegated writer verified `test -s desing/requirements.md` and zero fixes refining both OpenPencil frames; parent read back requirements, removed an unapproved extra rule about typed extensions, and visually inspected both previews including `s Stop`. No app runtime test.
- T6 done: read back requirements, US5 and non-goals; they consistently use fixed `D:\Music`, actionable missing-root error, and defer selection. No app runtime test.
- T7 done: read back playback requirements, US2, and open decisions; order is shared by track display and navigation, natural completion advances within the playback folder and stops at the end. No runtime test (planning-only).
- Next: verify Windows runtime compatibility before implementing; files remain uncommitted pending explicit authorization.
