package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/charmbracelet/huh"
	"github.com/orknist/dlpick/internal/ui"
	"github.com/orknist/dlpick/internal/ytdlp"
)

const usageText = `Usage:
  dlpick [options] [url]

dlpick asks whether you want video or audio, then lists the resolutions
and formats yt-dlp actually found for that URL.

Options:
  --video      Skip the first question and choose a video format
  --audio      Skip the first question and choose an audio format
  --dir DIR    Directory to save the file (default: current directory)
  -h, --help   Show this help

Examples:
  dlpick
  dlpick 'https://www.youtube.com/watch?v=...'
  dlpick --audio --dir ~/Music 'https://www.youtube.com/watch?v=...'
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "Cancelled.")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "dlpick: %s\n", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	fs := flag.NewFlagSet("dlpick", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", ".", "directory to save the download")
	video := fs.Bool("video", false, "choose a video format")
	audio := fs.Bool("audio", false, "choose an audio format")
	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stdout, usageText)
			return nil
		}
		fmt.Fprint(os.Stderr, usageText)
		return err
	}
	if *video && *audio {
		return errors.New("use only one of --video or --audio")
	}
	if fs.NArg() > 1 {
		fmt.Fprint(os.Stderr, usageText)
		return errors.New("too many arguments")
	}

	if !ytdlp.HasBinary("yt-dlp") {
		return errors.New("yt-dlp not found on PATH. Install it with: brew install yt-dlp")
	}

	mode := ""
	if *video {
		mode = "video"
	}
	if *audio {
		mode = "audio"
	}
	pageURL := ""
	if fs.NArg() == 1 {
		pageURL = fs.Arg(0)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return ui.Run(ctx, ui.Options{
		URL:  pageURL,
		Dir:  *dir,
		Mode: mode,
	})
}
