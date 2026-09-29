package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
	"github.com/orknist/dlpick/internal/ytdlp"
)

// Options configures one interactive run.
type Options struct {
	URL  string
	Dir  string
	Mode string
}

// Run asks for a format and downloads it with yt-dlp.
func Run(ctx context.Context, opts Options) error {
	if !isatty.IsTerminal(os.Stdin.Fd()) || !isatty.IsTerminal(os.Stdout.Fd()) {
		return errors.New("dlpick must be run in an interactive terminal")
	}
	if opts.Dir == "" {
		opts.Dir = "."
	}

	pageURL, err := resolveURL(ctx, opts.URL)
	if err != nil {
		return err
	}
	mode, err := resolveMode(ctx, opts.Mode)
	if err != nil {
		return err
	}

	var info *ytdlp.Info
	err = withSpinner(ctx, "Fetching formats...", func(ctx context.Context) error {
		var probeErr error
		info, probeErr = ytdlp.Probe(ctx, pageURL)
		return probeErr
	})
	if err != nil {
		return err
	}
	if len(info.Formats) == 0 {
		return errors.New("yt-dlp returned no formats for this URL")
	}

	playlist, err := resolvePlaylist(ctx, info)
	if err != nil {
		return err
	}

	var plan ytdlp.Plan
	switch mode {
	case "video":
		plan, err = pickVideo(ctx, info, playlist)
	case "audio":
		plan, err = pickAudio(ctx, info, playlist)
	default:
		err = fmt.Errorf("unknown mode %s", mode)
	}
	if err != nil {
		return err
	}
	plan, err = applySubtitles(ctx, info, plan, mode == "video")
	if err != nil {
		return err
	}

	saveDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return err
	}
	args := ytdlp.BuildArgs(pageURL, opts.Dir, playlist, plan)
	ok := true
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Start download?").
				Description(ytdlp.Summary(displayTitle(info), saveDir, plan, args, !ytdlp.HasBinary("ffmpeg"))).
				Affirmative("Download").
				Negative("Cancel").
				Value(&ok),
		),
	).RunWithContext(ctx)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "Cancelled.")
		return nil
	}

	if _, err := ytdlp.PrepareDir(opts.Dir); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr)
	return ytdlp.Download(ctx, args)
}

func resolveURL(ctx context.Context, pageURL string) (string, error) {
	pageURL = strings.TrimSpace(pageURL)
	if pageURL != "" {
		return pageURL, nil
	}
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("URL").
				Placeholder("https://...").
				Value(&pageURL).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("URL is required")
					}
					return nil
				}),
		),
	).RunWithContext(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(pageURL), nil
}

func resolveMode(ctx context.Context, mode string) (string, error) {
	switch mode {
	case "video", "audio":
		return mode, nil
	case "":
		mode = "video"
	default:
		return "", fmt.Errorf("unknown mode %s", mode)
	}
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What do you want to download?").
				Options(
					huh.NewOption("Video", "video"),
					huh.NewOption("Audio only", "audio"),
				).
				Value(&mode),
		),
	).RunWithContext(ctx)
	if err != nil {
		return "", err
	}
	return mode, nil
}

func resolvePlaylist(ctx context.Context, info *ytdlp.Info) (bool, error) {
	if !info.IsPlaylist() {
		return false, nil
	}
	scope := "one"
	err := askChoice(ctx, "This URL is a playlist", displayTitle(info), []ytdlp.Choice{
		{ID: "one", Label: "This video only"},
		{ID: "all", Label: "Whole playlist"},
	}, &scope)
	if err != nil {
		return false, err
	}
	return scope == "all", nil
}

func pickVideo(ctx context.Context, info *ytdlp.Info, playlist bool) (ytdlp.Plan, error) {
	resolutions := ytdlp.VideoResolutions(info.Formats)
	if len(resolutions) == 0 {
		return ytdlp.Plan{}, ytdlp.ErrNoVideoFormats
	}
	heightID := resolutions[0].ID
	if err := askChoice(ctx, "Resolution", displayTitle(info), resolutions, &heightID); err != nil {
		return ytdlp.Plan{}, err
	}
	height, err := strconv.Atoi(heightID)
	if err != nil {
		return ytdlp.Plan{}, fmt.Errorf("resolution %s: %w", heightID, err)
	}

	variants := ytdlp.VideoVariants(info.Formats, height)
	if len(variants) == 0 {
		return ytdlp.Plan{}, ytdlp.ErrNoVideoFormats
	}
	formatID := variants[0].ID
	if len(variants) > 1 {
		title := fmt.Sprintf("Format · %dp", height)
		if err := askChoice(ctx, title, displayTitle(info), variants, &formatID); err != nil {
			return ytdlp.Plan{}, err
		}
	}
	language := ""
	if !ytdlp.VideoHasOwnAudio(info.Formats, formatID) {
		var err error
		language, err = pickAudioLanguage(ctx, info)
		if err != nil {
			return ytdlp.Plan{}, err
		}
	}
	return ytdlp.PlanVideo(info.Formats, formatID, language, playlist)
}

func pickAudioLanguage(ctx context.Context, info *ytdlp.Info) (string, error) {
	languages := ytdlp.AudioLanguages(info.Formats)
	if len(languages) <= 1 {
		if len(languages) == 1 {
			return languages[0].ID, nil
		}
		return "", nil
	}
	language := languages[0].ID
	if err := askChoice(ctx, "Audio language", displayTitle(info), languages, &language); err != nil {
		return "", err
	}
	return language, nil
}

func pickAudio(ctx context.Context, info *ytdlp.Info, playlist bool) (ytdlp.Plan, error) {
	language, err := pickAudioLanguage(ctx, info)
	if err != nil {
		return ytdlp.Plan{}, err
	}
	originals := ytdlp.AudioOriginals(info.Formats, language)
	kinds := make([]ytdlp.Choice, 0, 5)
	if len(originals) > 0 {
		kinds = append(kinds, ytdlp.Choice{ID: "original", Label: "Original"})
	}
	kinds = append(kinds,
		ytdlp.Choice{ID: "mp3", Label: "MP3"},
		ytdlp.Choice{ID: "m4a", Label: "M4A (AAC)"},
		ytdlp.Choice{ID: "opus", Label: "Opus"},
		ytdlp.Choice{ID: "flac", Label: "FLAC"},
	)

	kind := kinds[0].ID
	if err := askChoice(ctx, "Audio format", displayTitle(info), kinds, &kind); err != nil {
		return ytdlp.Plan{}, err
	}
	if kind == "original" {
		formatID := originals[0].ID
		if len(originals) > 1 {
			if err := askChoice(ctx, "Quality", displayTitle(info), originals, &formatID); err != nil {
				return ytdlp.Plan{}, err
			}
		}
		return ytdlp.PlanAudioOriginal(info.Formats, formatID, playlist)
	}

	bitrates := ytdlp.AudioBitrates(info.Formats, language)
	quality := "0"
	if len(bitrates) > 1 {
		if err := askChoice(ctx, "Quality", displayTitle(info), bitrates, &quality); err != nil {
			return ytdlp.Plan{}, err
		}
	}
	plan, err := ytdlp.PlanAudioConvert(kind, quality, language)
	if err != nil {
		return ytdlp.Plan{}, err
	}
	return appendLanguage(plan, language, info.Formats), nil
}

func appendLanguage(plan ytdlp.Plan, language string, formats []ytdlp.Format) ytdlp.Plan {
	if language == "" {
		return plan
	}
	for _, choice := range ytdlp.AudioLanguages(formats) {
		if choice.ID == language {
			plan.Label += " · " + choice.Label
			return plan
		}
	}
	return plan
}

func applySubtitles(ctx context.Context, info *ytdlp.Info, plan ytdlp.Plan, embed bool) (ytdlp.Plan, error) {
	choices := info.SubtitleChoices()
	if len(choices) == 0 {
		return plan, nil
	}
	selected := ""
	if err := askChoice(ctx, "Subtitles", displayTitle(info), choices, &selected); err != nil {
		return ytdlp.Plan{}, err
	}
	if selected == "" {
		return plan, nil
	}
	label := ""
	for _, choice := range choices {
		if choice.ID == selected {
			label = choice.Label
			break
		}
	}
	if embed {
		place := "embed"
		err := askChoice(ctx, "Save subtitles", displayTitle(info), []ytdlp.Choice{
			{ID: "embed", Label: "Embed in the video"},
			{ID: "srt", Label: "Save as .srt"},
		}, &place)
		if err != nil {
			return ytdlp.Plan{}, err
		}
		embed = place == "embed"
	}
	return plan.WithSubtitle(selected, label, embed), nil
}

func askChoice(ctx context.Context, title, description string, choices []ytdlp.Choice, value *string) error {
	if len(choices) == 0 {
		return errors.New("nothing to choose")
	}
	if *value == "" {
		*value = choices[0].ID
	}
	options := make([]huh.Option[string], len(choices))
	for i, choice := range choices {
		options[i] = huh.NewOption(choice.Label, choice.ID)
	}
	field := huh.NewSelect[string]().
		Title(title).
		Options(options...).
		Value(value)
	if description != "" {
		field.Description(description)
	}
	if len(choices) > 8 {
		field.Height(8)
	}
	return huh.NewForm(huh.NewGroup(field)).RunWithContext(ctx)
}

func displayTitle(info *ytdlp.Info) string {
	if info == nil || info.Title == "" {
		return ""
	}
	return info.Title
}
