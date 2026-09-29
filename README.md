# dlpick

dlpick is a terminal picker for [yt-dlp](https://github.com/yt-dlp/yt-dlp). It asks whether you want video or audio, then lists the resolutions, codecs, languages, and subtitles that URL actually has.

dlpick does not download the media itself. After you confirm, it runs `yt-dlp` and leaves the progress output on the terminal.

## Install with Homebrew

```sh
brew install orknist/tap/dlpick
```

This also installs `yt-dlp` and `ffmpeg`. Upgrade later with:

```sh
brew update
brew upgrade dlpick
```

## Build from source

You need Go 1.27 or newer, plus `yt-dlp` and `ffmpeg` on your `PATH`.

```sh
git clone https://github.com/orknist/dlpick.git
cd dlpick
go build -o dlpick ./cmd/dlpick
```

Put the binary somewhere on your `PATH`, or run `./dlpick` from that directory.

On macOS, the two external tools are:

```sh
brew install yt-dlp ffmpeg
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
| `-h`, `--help` | Show help |

Arrow keys move, Enter selects, `/` filters a long list, and Esc cancels.

If `yt-dlp` is missing, dlpick stops and tells you to install it. `ffmpeg` is required when a video stream has separate audio, or when you convert audio to another format.

## What it asks

Video choices are grouped by resolution, then by container and codec. When a video has more than one spoken language, dlpick asks which one to keep and lists the original track first. A video-only stream is paired with matching audio: MP4 with AAC, WebM with Opus. A single video is downloaded with those format ids. A playlist uses a height, codec, and language selector, because one format id does not apply to every entry.

If subtitles exist, dlpick asks which language to save. For a video, it then asks whether to embed them or save a separate `.srt`. Uploaded subtitles are preferred. Auto-generated captions are offered only when the video has no uploaded subtitles. An audio download keeps subtitles as a `.srt` beside the file.

Audio can stay in the original stream, or be converted to MP3, M4A, Opus, or FLAC. YouTube's DRC copies are skipped when a normal audio track exists.

Files are saved as `%(title)s [%(id)s].%(ext)s`.

## License

[MIT](LICENSE)
