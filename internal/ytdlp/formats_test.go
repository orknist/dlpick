package ytdlp

import (
	"strings"
	"testing"
)

func sampleFormats() []Format {
	return []Format{
		{FormatID: "sb0", Ext: "mhtml", FormatNote: "storyboard", VCodec: "none", ACodec: "none"},
		{FormatID: "139", Ext: "m4a", ACodec: "mp4a.40.5", VCodec: "none", ABR: 48, Protocol: "https", Filesize: 1000},
		{FormatID: "140", Ext: "m4a", ACodec: "mp4a.40.2", VCodec: "none", ABR: 128, Protocol: "https", Filesize: 3000},
		{FormatID: "140-tr", Ext: "m4a", ACodec: "mp4a.40.2", VCodec: "none", ABR: 128, Protocol: "https", Filesize: 3000, Language: "tr"},
		{FormatID: "140-drc", Ext: "m4a", ACodec: "mp4a.40.2", VCodec: "none", ABR: 128, Protocol: "https", FormatNote: "DRC", Filesize: 3000},
		{FormatID: "251", Ext: "webm", ACodec: "opus", VCodec: "none", ABR: 160, Protocol: "https", Filesize: 4000},
		{FormatID: "251-drc", Ext: "webm", ACodec: "opus", VCodec: "none", ABR: 170, Protocol: "https", FormatNote: "DRC", Filesize: 4100},
		{FormatID: "137", Ext: "mp4", VCodec: "avc1.640028", ACodec: "none", Height: 1080, TBR: 4500, Protocol: "https", Filesize: 50_000_000},
		{FormatID: "399", Ext: "mp4", VCodec: "avc1.640028", ACodec: "none", Height: 1080, TBR: 4500, Protocol: "m3u8_native", Filesize: 50_000_000},
		{FormatID: "248", Ext: "webm", VCodec: "vp9", ACodec: "none", Height: 1080, TBR: 3000, Protocol: "https", Filesize: 40_000_000, DynamicRange: "SDR"},
		{FormatID: "303", Ext: "webm", VCodec: "vp9.2", ACodec: "none", Height: 1080, TBR: 5000, Protocol: "https", Filesize: 60_000_000, DynamicRange: "HDR10", HasDRM: true},
		{FormatID: "22", Ext: "mp4", VCodec: "avc1.64001F", ACodec: "mp4a.40.2", Height: 720, TBR: 800, Protocol: "https", Filesize: 20_000_000},
		{FormatID: "271", Ext: "webm", VCodec: "vp9", ACodec: "none", Height: 1440, TBR: 9000, Protocol: "https"},
	}
}

func TestVideoResolutionsSkipStoryboardAndDRM(t *testing.T) {
	choices := VideoResolutions(sampleFormats())
	if len(choices) != 3 {
		t.Fatalf("resolutions = %#v", choices)
	}
	got := []string{choices[0].ID, choices[1].ID, choices[2].ID}
	want := []string{"1440", "1080", "720"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("heights = %v, want %v", got, want)
		}
	}
	if !strings.Contains(choices[1].Label, "2 options") {
		t.Fatalf("1080p label = %q", choices[1].Label)
	}
}

func TestVideoVariantsPreferHTTPSAndH264(t *testing.T) {
	choices := VideoVariants(sampleFormats(), 1080)
	if len(choices) != 2 {
		t.Fatalf("variants = %#v", choices)
	}
	if choices[0].ID != "137" {
		t.Fatalf("first variant = %s, want 137 (https H.264, not the HLS duplicate)", choices[0].ID)
	}
	if !strings.Contains(choices[0].Label, "MP4") || !strings.Contains(choices[0].Label, "H.264") {
		t.Fatalf("h264 label = %q", choices[0].Label)
	}
	if choices[1].ID != "248" {
		t.Fatalf("second variant = %s, want 248", choices[1].ID)
	}
}

func TestPlanVideoPairsCompatibleAudio(t *testing.T) {
	plan, err := PlanVideo(sampleFormats(), "137", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Selector[1]; got != "137+140" {
		t.Fatalf("selector = %s, want 137+140", got)
	}
	if !strings.Contains(plan.Label, "AAC audio") {
		t.Fatalf("label = %q", plan.Label)
	}
	if plan.EstimatedSize != 50_003_000 {
		t.Fatalf("size = %d", plan.EstimatedSize)
	}
	if !plan.NeedsFFmpeg() {
		t.Fatal("merged video should need ffmpeg")
	}
}

func TestPlanVideoKeepsProgressiveFormat(t *testing.T) {
	plan, err := PlanVideo(sampleFormats(), "22", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Selector[1] != "22" {
		t.Fatalf("selector = %s", plan.Selector[1])
	}
	if plan.NeedsFFmpeg() {
		t.Fatal("progressive format should not need ffmpeg")
	}
}

func TestPlanVideoPlaylistUsesGenericSelector(t *testing.T) {
	plan, err := PlanVideo(sampleFormats(), "137", "", true)
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Selector[1]
	if strings.Contains(got, "137") || !strings.Contains(got, "height=1080") || !strings.Contains(got, "vcodec^=avc1") || !strings.Contains(got, "+ba[ext=m4a]") {
		t.Fatalf("playlist selector = %s", got)
	}
}

func TestPlanVideoWebMPairsOpus(t *testing.T) {
	plan, err := PlanVideo(sampleFormats(), "248", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Selector[1] != "248+251" {
		t.Fatalf("selector = %s, want 248+251", plan.Selector[1])
	}
}

func TestAudioOriginalsOrder(t *testing.T) {
	choices := AudioOriginals(sampleFormats(), "")
	if len(choices) != 4 {
		t.Fatalf("audio choices = %#v", choices)
	}
	if choices[0].ID != "251" {
		t.Fatalf("first audio = %s, want 160kbps opus", choices[0].ID)
	}
	if choices[1].ID != "140" {
		t.Fatalf("second audio = %s, want default 128kbps aac", choices[1].ID)
	}
	for _, choice := range choices {
		if strings.Contains(choice.ID, "drc") {
			t.Fatalf("DRC listed next to a normal track: %#v", choices)
		}
	}
}

func TestPlanAudioOriginalAndConvert(t *testing.T) {
	original, err := PlanAudioOriginal(sampleFormats(), "251", false)
	if err != nil {
		t.Fatal(err)
	}
	if original.Selector[1] != "251" {
		t.Fatalf("original selector = %s", original.Selector[1])
	}
	if original.NeedsFFmpeg() {
		t.Fatal("original audio should not need ffmpeg")
	}

	playlist, err := PlanAudioOriginal(sampleFormats(), "251", true)
	if err != nil {
		t.Fatal(err)
	}
	if playlist.Selector[1] != "ba[ext=webm][acodec^=opus][abr<=160]/ba" {
		t.Fatalf("playlist audio selector = %s", playlist.Selector[1])
	}

	converted, err := PlanAudioConvert("mp3", "0", "")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(converted.Selector, " ")
	if joined != "-f bestaudio/best -x --audio-format mp3 --audio-quality 0" {
		t.Fatalf("convert selector = %s", joined)
	}
	if !converted.NeedsFFmpeg() {
		t.Fatal("conversion should need ffmpeg")
	}

	rated, err := PlanAudioConvert("opus", "128K", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rated.Selector, " "), "--audio-quality 128K") {
		t.Fatalf("rated selector = %v", rated.Selector)
	}
	if rated.Label != "OPUS · 128 kbps" {
		t.Fatalf("label = %q", rated.Label)
	}
}

func TestAudioBitrates(t *testing.T) {
	choices := AudioBitrates(sampleFormats(), "")
	if choices[0].ID != "0" || choices[0].Label != "Best" {
		t.Fatalf("first bitrate = %#v", choices[0])
	}
	ids := make([]string, len(choices))
	for i, choice := range choices {
		ids[i] = choice.ID
	}
	if strings.Join(ids, ",") != "0,160K,128K,48K" {
		t.Fatalf("bitrates = %v", ids)
	}
}

func TestAudioBitratesCollapseCloseRates(t *testing.T) {
	formats := []Format{
		{FormatID: "a", Ext: "webm", ACodec: "opus", VCodec: "none", ABR: 107},
		{FormatID: "b", Ext: "webm", ACodec: "opus", VCodec: "none", ABR: 106},
		{FormatID: "c", Ext: "m4a", ACodec: "mp4a.40.2", VCodec: "none", ABR: 130},
	}
	var ids []string
	for _, choice := range AudioBitrates(formats, "") {
		ids = append(ids, choice.ID)
	}
	if strings.Join(ids, ",") != "0,128K,96K" {
		t.Fatalf("bitrates = %v", ids)
	}
}

func TestBuildArgsAndSummary(t *testing.T) {
	plan, err := PlanVideo(sampleFormats(), "137", "", false)
	if err != nil {
		t.Fatal(err)
	}
	args := BuildArgs("https://example.com/watch?v=abc", ".", false, plan)
	if args[0] != "--no-playlist" || args[len(args)-1] != "https://example.com/watch?v=abc" {
		t.Fatalf("args = %#v", args)
	}
	quoted := QuoteCommand(args)
	if !strings.HasPrefix(quoted, "yt-dlp --no-playlist -f 137+140 -o ") {
		t.Fatalf("command = %s", quoted)
	}
	if !strings.Contains(quoted, "'%(title)s [%(id)s].%(ext)s'") {
		t.Fatalf("output template not quoted: %s", quoted)
	}

	playlist := BuildArgs("https://example.com/playlist", "/tmp/dl", true, plan)
	if playlist[0] == "--no-playlist" {
		t.Fatalf("playlist args should omit --no-playlist: %#v", playlist)
	}
	output := ""
	for i, arg := range playlist {
		if arg == "-o" && i+1 < len(playlist) {
			output = playlist[i+1]
		}
	}
	if !strings.Contains(output, "/tmp/dl") {
		t.Fatalf("output path = %q", output)
	}

	summary := Summary("Demo", "/tmp/dl", plan, args, true)
	if !strings.Contains(summary, "Title: Demo") || !strings.Contains(summary, "ffmpeg was not found") {
		t.Fatalf("summary = %s", summary)
	}
	if FormatBytes(50_000_000) != "47.7 MB" {
		t.Fatalf("size = %s", FormatBytes(50_000_000))
	}
}

func TestParseInfoPlaylistFallsBackToFirstEntry(t *testing.T) {
	raw := []byte(`notice
{"_type":"playlist","title":"Shows","entries":[
  {"id":"a","title":"One","formats":[{"format_id":"18","ext":"mp4","height":360,"vcodec":"avc1","acodec":"mp4a.40.2"}]},
  {"id":"b","title":"Two"}
]}`)
	info, err := ParseInfo(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsPlaylist() {
		t.Fatal("expected a playlist")
	}
	if info.PlaylistCount != 2 {
		t.Fatalf("playlist count = %d", info.PlaylistCount)
	}
	if info.Title != "Shows" {
		t.Fatalf("title = %s", info.Title)
	}
	if len(info.Formats) != 1 || info.Formats[0].FormatID != "18" {
		t.Fatalf("formats = %#v", info.Formats)
	}
}

func TestParseInfoSingleVideoPlaylistCount(t *testing.T) {
	raw := []byte(`{"id":"abc","title":"Clip","playlist_count":8,"formats":[{"format_id":"18","ext":"mp4","height":360,"vcodec":"avc1","acodec":"mp4a.40.2"}]}`)
	info, err := ParseInfo(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsPlaylist() || info.Title != "Clip" {
		t.Fatalf("info = %#v", info)
	}
}

func dubbedFormats() []Format {
	return []Format{
		{FormatID: "137", Ext: "mp4", VCodec: "avc1.640028", ACodec: "none", Height: 1080, Protocol: "https"},
		{FormatID: "248", Ext: "webm", VCodec: "vp9", ACodec: "none", Height: 1080, Protocol: "https"},
		{FormatID: "140-0", Ext: "m4a", ACodec: "mp4a.40.2", VCodec: "none", ABR: 129, Language: "ja", FormatNote: "Japanese, medium", Protocol: "https"},
		{FormatID: "140-3", Ext: "m4a", ACodec: "mp4a.40.2", VCodec: "none", ABR: 129, Language: "tr", FormatNote: "Turkish, medium", Protocol: "https"},
		{FormatID: "140-5", Ext: "m4a", ACodec: "mp4a.40.2", VCodec: "none", ABR: 129, Language: "en", FormatNote: "English original (default), medium", LanguagePreference: 10, Protocol: "https"},
		{FormatID: "251-1", Ext: "webm", ACodec: "opus", VCodec: "none", ABR: 110, Language: "ja", FormatNote: "Japanese, medium", Protocol: "https"},
		{FormatID: "251-5", Ext: "webm", ACodec: "opus", VCodec: "none", ABR: 125, Language: "en", FormatNote: "English original (default), medium", LanguagePreference: 10, Protocol: "https"},
		{FormatID: "233-0", Ext: "mp4", ACodec: "mp4a.40.2", VCodec: "none", Language: "ja", FormatNote: "日本語 - dubbed", Protocol: "m3u8"},
	}
}

func TestAudioLanguagesPreferOriginal(t *testing.T) {
	choices := AudioLanguages(dubbedFormats())
	if len(choices) != 3 {
		t.Fatalf("languages = %#v", choices)
	}
	if choices[0].ID != "en" || choices[0].Label != "English (original)" {
		t.Fatalf("first language = %#v", choices[0])
	}
	if choices[1].ID != "ja" || choices[1].Label != "Japanese" {
		t.Fatalf("second language = %#v", choices[1])
	}
	if choices[2].ID != "tr" || choices[2].Label != "Turkish" {
		t.Fatalf("third language = %#v", choices[2])
	}
}

func TestPlanVideoUsesChosenLanguage(t *testing.T) {
	formats := dubbedFormats()

	fallback, err := PlanVideo(formats, "137", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.Selector[1] != "137+140-5" {
		t.Fatalf("fallback selector = %s, want English original, not the first format id", fallback.Selector[1])
	}

	turkish, err := PlanVideo(formats, "137", "tr", false)
	if err != nil {
		t.Fatal(err)
	}
	if turkish.Selector[1] != "137+140-3" {
		t.Fatalf("turkish selector = %s", turkish.Selector[1])
	}
	if !strings.Contains(turkish.Label, "Turkish audio") {
		t.Fatalf("label = %q", turkish.Label)
	}

	japanese, err := PlanVideo(formats, "248", "ja", false)
	if err != nil {
		t.Fatal(err)
	}
	if japanese.Selector[1] != "248+251-1" {
		t.Fatalf("japanese webm selector = %s", japanese.Selector[1])
	}

	playlist, err := PlanVideo(formats, "137", "ja", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(playlist.Selector[1], "language=ja") {
		t.Fatalf("playlist selector = %s", playlist.Selector[1])
	}
}

func TestPlanAudioConvertKeepsLanguage(t *testing.T) {
	plan, err := PlanAudioConvert("mp3", "0", "tr")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Selector[1] != "bestaudio[language=tr]/best[language=tr]/bestaudio/best" {
		t.Fatalf("selector = %s", plan.Selector[1])
	}
}

func TestSubtitleChoicesPreferManual(t *testing.T) {
	info := Info{
		Subtitles: map[string][]Caption{
			"en":        {{Ext: "vtt", Name: "English"}},
			"tr":        {{Ext: "srt", Name: "Turkish"}},
			"live_chat": {{Ext: "json", Name: "Live chat"}},
		},
		AutomaticCaptions: map[string][]Caption{
			"de": {{Ext: "vtt", Name: "German"}},
		},
	}
	choices := info.SubtitleChoices()
	var ids []string
	for _, choice := range choices {
		ids = append(ids, choice.ID+"="+choice.Label)
	}
	if strings.Join(ids, ",") != "=No subtitles,en=English,tr=Turkish" {
		t.Fatalf("subtitles = %v", ids)
	}

	autoOnly := Info{AutomaticCaptions: map[string][]Caption{
		"de": {{Ext: "vtt", Name: "German"}},
	}}
	auto := autoOnly.SubtitleChoices()
	if len(auto) != 2 || auto[1].ID != "auto:de" || auto[1].Label != "German (auto)" {
		t.Fatalf("auto subtitles = %#v", auto)
	}

	plan := Plan{Label: "1080p", Selector: []string{"-f", "137+140-3"}}
	withSubs := plan.WithSubtitle("tr", "Turkish", true)
	joined := strings.Join(withSubs.Selector, " ")
	if joined != "-f 137+140-3 --write-subs --sub-langs tr --embed-subs --compat-options no-keep-subs" {
		t.Fatalf("subtitle args = %s", joined)
	}
	if !strings.Contains(withSubs.Label, "Turkish subtitles, embedded") || !withSubs.NeedsFFmpeg() {
		t.Fatalf("plan = %#v", withSubs)
	}
	asFile := plan.WithSubtitle("tr", "Turkish", false)
	if strings.Join(asFile.Selector, " ") != "-f 137+140-3 --write-subs --sub-langs tr --convert-subs srt" {
		t.Fatalf("srt args = %s", strings.Join(asFile.Selector, " "))
	}
	if !strings.Contains(asFile.Label, "Turkish subtitles (.srt)") {
		t.Fatalf("srt label = %q", asFile.Label)
	}
	if plan.WithSubtitle("", "No subtitles", true).Label != "1080p" {
		t.Fatal("empty subtitle id should leave the plan unchanged")
	}
}
