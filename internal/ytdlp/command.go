package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const outputTemplate = "%(title)s [%(id)s].%(ext)s"

// Info is the yt-dlp metadata used to build the prompts.
type Info struct {
	ID                string               `json:"id"`
	Title             string               `json:"title"`
	Type              string               `json:"_type"`
	PlaylistCount     int                  `json:"playlist_count"`
	Formats           []Format             `json:"formats"`
	Entries           []Info               `json:"entries"`
	Subtitles         map[string][]Caption `json:"subtitles"`
	AutomaticCaptions map[string][]Caption `json:"automatic_captions"`
}

// Caption is one subtitle track in a yt-dlp JSON dump.
type Caption struct {
	Ext  string `json:"ext"`
	Name string `json:"name"`
}

// IsPlaylist reports whether the URL points at more than one video.
func (i Info) IsPlaylist() bool {
	if i.PlaylistCount > 1 {
		return true
	}
	return i.Type == "playlist" && len(i.Entries) > 1
}

// HasBinary reports whether name is on PATH.
func HasBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Probe loads format metadata for one video.
// Playlist URLs are flattened to a single entry so the prompt stays small.
// playlist_count is preserved and used to ask about the rest of the list.
func Probe(ctx context.Context, pageURL string) (*Info, error) {
	cmd := exec.CommandContext(ctx, "yt-dlp",
		"-J",
		"--no-download",
		"--no-playlist",
		"--no-warnings",
		pageURL,
	)
	configureCommand(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 4000 {
			msg = msg[len(msg)-4000:]
		}
		return nil, fmt.Errorf("yt-dlp: %s", msg)
	}
	return ParseInfo(stdout.Bytes())
}

// ParseInfo decodes a yt-dlp JSON dump.
func ParseInfo(data []byte) (*Info, error) {
	data = extractJSON(data)
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("read yt-dlp metadata: %w", err)
	}
	normalize(&info)
	if info.Title == "" {
		info.Title = info.ID
	}
	return &info, nil
}

// PrepareDir creates dir when needed and returns its absolute path.
func PrepareDir(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	return abs, nil
}

// BuildArgs assembles the yt-dlp invocation for a confirmed plan.
func BuildArgs(pageURL, dir string, playlist bool, plan Plan) []string {
	args := make([]string, 0, 6+len(plan.Selector))
	if !playlist {
		args = append(args, "--no-playlist")
	}
	args = append(args, plan.Selector...)
	output := outputTemplate
	if dir != "" && dir != "." {
		output = filepath.Join(dir, outputTemplate)
	}
	args = append(args, "-o", output, pageURL)
	return args
}

// Summary is the confirmation text shown before download.
func Summary(title, dir string, plan Plan, args []string, ffmpegMissing bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", title)
	fmt.Fprintf(&b, "Selection: %s\n", plan.Label)
	if size := FormatBytes(plan.EstimatedSize); size != "" {
		fmt.Fprintf(&b, "Estimated size: %s\n", size)
	}
	fmt.Fprintf(&b, "Save to: %s\n\n", dir)
	b.WriteString(QuoteCommand(args))
	if ffmpegMissing && plan.NeedsFFmpeg() {
		b.WriteString("\n\nffmpeg was not found. This selection needs it to merge or convert (brew install ffmpeg).")
	}
	return b.String()
}

// QuoteCommand renders argv as a copyable shell command.
func QuoteCommand(args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "yt-dlp")
	for _, arg := range args {
		parts = append(parts, quoteArg(arg))
	}
	return strings.Join(parts, " ")
}

// Download runs yt-dlp and leaves its progress on the terminal.
func Download(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	configureCommand(cmd)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("yt-dlp exited with status %d", exitErr.ExitCode())
		}
		return err
	}
	return nil
}

func normalize(info *Info) {
	if len(info.Formats) == 0 {
		for _, entry := range info.Entries {
			if len(entry.Formats) == 0 {
				continue
			}
			if info.PlaylistCount == 0 && len(info.Entries) > 1 {
				info.PlaylistCount = len(info.Entries)
			}
			if info.Title == "" {
				info.Title = entry.Title
			}
			if info.ID == "" {
				info.ID = entry.ID
			}
			info.Formats = entry.Formats
			if len(info.Subtitles) == 0 {
				info.Subtitles = entry.Subtitles
			}
			if len(info.AutomaticCaptions) == 0 {
				info.AutomaticCaptions = entry.AutomaticCaptions
			}
			break
		}
	}
	if info.Type == "playlist" && info.PlaylistCount == 0 && len(info.Entries) > 0 {
		info.PlaylistCount = len(info.Entries)
	}
}

func extractJSON(data []byte) []byte {
	start := bytes.IndexByte(data, '{')
	end := bytes.LastIndexByte(data, '}')
	if start >= 0 && end > start {
		return data[start : end+1]
	}
	return data
}

func quoteArg(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("-_./:=@+", r) {
			continue
		}
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}
