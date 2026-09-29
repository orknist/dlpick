# dlpick

A small terminal picker for [yt-dlp](https://github.com/yt-dlp/yt-dlp). It asks whether you want video or audio, then lists the resolutions and formats that URL actually has.

dlpick does not download anything itself. After you confirm, it runs `yt-dlp` and leaves the progress bar on the terminal.

## Dependencies

- `yt-dlp`
- `ffmpeg`, when the chosen video has a separate audio stream or when audio is converted

```sh
brew install yt-dlp ffmpeg
```

## Build

```sh
go build -o dlpick ./cmd/dlpick
```

## Usage

```sh
dlpick
dlpick 'https://www.youtube.com/watch?v=...'
dlpick --audio --dir ~/Music 'https://www.youtube.com/watch?v=...'
```

| Flag | Effect |
| --- | --- |
| `--video` | Skip the first question and choose a video format |
| `--audio` | Skip the first question and choose an audio format |
| `--dir DIR` | Save the file here instead of the current directory |

Arrow keys move, enter selects, `/` filters a long list, and esc cancels.

Video choices are grouped by resolution, then by container and codec. When the video has more than one spoken language, dlpick asks which one to keep. The original track is listed first. A video-only stream is then paired with the matching audio (MP4 with AAC, WebM with Opus). One video is downloaded with those format ids. A whole playlist uses a height, codec, and language selector, because a single format id does not apply to every entry.

If subtitles exist, dlpick asks which language to save. For a video, it then asks whether to embed them or save a separate `.srt`. Uploaded subtitles are preferred. Auto-generated captions are offered only when the video has no uploaded subtitles. Audio downloads keep subtitles as a `.srt` beside the file.

Audio can stay in the original stream, or be converted to MP3, M4A, Opus, or FLAC. YouTube's DRC copies are skipped when a normal audio track exists.

Files are saved as `%(title)s [%(id)s].%(ext)s`.

## Homebrew

Not published yet. A later formula should depend on `yt-dlp` and `ffmpeg` and install this binary as `dlpick`.
