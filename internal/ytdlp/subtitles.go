package ytdlp

import (
	"sort"
	"strings"
)

// SubtitleChoices lists subtitle languages for the prompt.
// The first row is always "No subtitles" when any language exists.
// Uploaded subtitles win over auto-generated captions for the same language.
// Auto-generated captions are listed only when the video has no uploaded subtitles,
// because YouTube can attach more than a hundred automatic languages.
func (i Info) SubtitleChoices() []Choice {
	manual := subtitleRows(i.Subtitles, false)
	if len(manual) > 0 {
		return append([]Choice{{ID: "", Label: "No subtitles"}}, manual...)
	}
	auto := subtitleRows(i.AutomaticCaptions, true)
	if len(auto) == 0 {
		return nil
	}
	return append([]Choice{{ID: "", Label: "No subtitles"}}, auto...)
}

// WithSubtitle adds subtitle flags to a download plan.
// An empty id leaves the plan unchanged. embed puts the subtitles inside a video file.
func (p Plan) WithSubtitle(id, label string, embed bool) Plan {
	if id == "" {
		return p
	}
	lang := id
	writeFlag := "--write-subs"
	if strings.HasPrefix(id, "auto:") {
		lang = strings.TrimPrefix(id, "auto:")
		writeFlag = "--write-auto-subs"
	}
	args := []string{writeFlag, "--sub-langs", lang}
	suffix := "subtitles (.srt)"
	if embed {
		// Leave the subtitle in its original container format. WebM can only
		// embed WebVTT, and converting to SRT first makes that embed fail.
		// yt-dlp keeps the sidecar unless no-keep-subs is set.
		args = append(args, "--embed-subs", "--compat-options", "no-keep-subs")
		suffix = "subtitles, embedded"
	} else {
		args = append(args, "--convert-subs", "srt")
	}
	p.Selector = append(append([]string{}, p.Selector...), args...)
	if label != "" {
		p.Label += " + " + label + " " + suffix
	}
	return p
}

func subtitleRows(tracks map[string][]Caption, auto bool) []Choice {
	choices := make([]Choice, 0, len(tracks))
	for code, captions := range tracks {
		if code == "" || code == "live_chat" {
			continue
		}
		label := captionName(captions)
		if label == "" {
			label = code
		}
		id := code
		if auto {
			label += " (auto)"
			id = "auto:" + code
		}
		choices = append(choices, Choice{ID: id, Label: label})
	}
	sort.Slice(choices, func(i, j int) bool {
		return strings.ToLower(choices[i].Label) < strings.ToLower(choices[j].Label)
	})
	return choices
}

func captionName(captions []Caption) string {
	for _, caption := range captions {
		if name := strings.TrimSpace(caption.Name); name != "" {
			return name
		}
	}
	return ""
}
