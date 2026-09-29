package ytdlp

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Format is one stream reported by yt-dlp's JSON dump.
type Format struct {
	FormatID           string  `json:"format_id"`
	Ext                string  `json:"ext"`
	Height             int     `json:"height"`
	VCodec             string  `json:"vcodec"`
	ACodec             string  `json:"acodec"`
	Filesize           int64   `json:"filesize"`
	FilesizeApprox     int64   `json:"filesize_approx"`
	TBR                float64 `json:"tbr"`
	VBR                float64 `json:"vbr"`
	ABR                float64 `json:"abr"`
	FormatNote         string  `json:"format_note"`
	Protocol           string  `json:"protocol"`
	DynamicRange       string  `json:"dynamic_range"`
	Language           string  `json:"language"`
	LanguagePreference int     `json:"language_preference"`
	HasDRM             bool    `json:"has_drm"`
}

// Choice is one row in a prompt.
type Choice struct {
	ID    string
	Label string
}

// Plan is the format selection translated into yt-dlp arguments.
type Plan struct {
	Label         string
	EstimatedSize int64
	// Selector is the format portion of the yt-dlp command, such as
	// ["-f", "137+140"] or the audio-extraction flags.
	Selector []string
}

// ErrNoVideoFormats is returned when a URL exposes no downloadable video.
var ErrNoVideoFormats = fmt.Errorf("no video formats found for this URL")

func (f Format) Size() int64 {
	if f.Filesize > 0 {
		return f.Filesize
	}
	return f.FilesizeApprox
}

func (f Format) HasVideo() bool {
	return f.VCodec != "" && !strings.EqualFold(f.VCodec, "none")
}

func (f Format) HasAudio() bool {
	return f.ACodec != "" && !strings.EqualFold(f.ACodec, "none")
}

func (f Format) AudioOnly() bool {
	return f.HasAudio() && !f.HasVideo()
}

// VideoResolutions lists heights that exist on this URL, tallest first.
func VideoResolutions(formats []Format) []Choice {
	heights := map[int]struct{}{}
	for _, f := range videoCandidates(formats) {
		heights[f.Height] = struct{}{}
	}
	ordered := make([]int, 0, len(heights))
	for height := range heights {
		ordered = append(ordered, height)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ordered)))

	choices := make([]Choice, 0, len(ordered))
	for _, height := range ordered {
		n := len(videoVariants(formats, height))
		label := fmt.Sprintf("%dp", height)
		if n > 1 {
			label = fmt.Sprintf("%dp · %d options", height, n)
		}
		choices = append(choices, Choice{ID: strconv.Itoa(height), Label: label})
	}
	return choices
}

// VideoVariants lists container and codec choices at one height.
func VideoVariants(formats []Format, height int) []Choice {
	variants := videoVariants(formats, height)
	choices := make([]Choice, 0, len(variants))
	for _, f := range variants {
		choices = append(choices, Choice{ID: f.FormatID, Label: videoVariantLabel(f)})
	}
	return choices
}

// AudioOriginals lists separate audio streams, highest bitrate first.
// language limits the list to one spoken language. An empty language keeps every track.
func AudioOriginals(formats []Format, language string) []Choice {
	audio := audioCandidatesFor(formats, language)
	sort.SliceStable(audio, func(i, j int) bool {
		return audioListBetter(audio[i], audio[j])
	})
	choices := make([]Choice, 0, len(audio))
	for _, f := range audio {
		choices = append(choices, Choice{ID: f.FormatID, Label: audioChoiceLabel(f)})
	}
	return choices
}

// AudioBitrates lists conversion qualities. The first row is always best.
// Bitrates come from separate audio streams. A muxed file's total bitrate
// is not an audio quality, so it is used only when no audio stream exists.
func AudioBitrates(formats []Format, language string) []Choice {
	sources := audioCandidatesFor(formats, language)
	if len(sources) == 0 {
		for _, f := range formats {
			if f.HasDRM || f.storyboard() || !f.HasAudio() {
				continue
			}
			sources = append(sources, f)
		}
	}

	seen := map[int]struct{}{}
	var rates []int
	for _, f := range sources {
		kbps := bucketBitrate(bitrateKbps(f))
		if kbps <= 0 {
			continue
		}
		if _, ok := seen[kbps]; ok {
			continue
		}
		seen[kbps] = struct{}{}
		rates = append(rates, kbps)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(rates)))

	choices := make([]Choice, 0, len(rates)+1)
	choices = append(choices, Choice{ID: "0", Label: "Best"})
	for _, kbps := range rates {
		choices = append(choices, Choice{
			ID:    fmt.Sprintf("%dK", kbps),
			Label: fmt.Sprintf("%d kbps", kbps),
		})
	}
	return choices
}

// AudioLanguages lists spoken languages on the separate audio streams.
// A single language is still returned so the caller can pin the download to it.
// No choices means the tracks are not tagged with a language.
func AudioLanguages(formats []Format) []Choice {
	type langInfo struct {
		name     string
		original bool
	}
	found := map[string]*langInfo{}
	var codes []string
	unlabeled := false
	for _, f := range audioCandidates(formats) {
		if f.Language == "" {
			unlabeled = true
			continue
		}
		info, ok := found[f.Language]
		if !ok {
			info = &langInfo{}
			found[f.Language] = info
			codes = append(codes, f.Language)
		}
		if isOriginalAudio(f) {
			info.original = true
		}
		info.name = preferLanguageName(info.name, cleanLanguageNote(f.FormatNote))
	}
	if unlabeled && len(found) > 0 {
		found["default"] = &langInfo{name: "Default"}
		codes = append(codes, "default")
	}
	if len(found) == 0 {
		return nil
	}

	type row struct {
		choice   Choice
		original bool
	}
	rows := make([]row, 0, len(codes))
	for _, code := range codes {
		info := found[code]
		label := info.name
		if label == "" {
			label = code
		}
		if info.original && !strings.Contains(strings.ToLower(label), "original") {
			label += " (original)"
		}
		rows = append(rows, row{choice: Choice{ID: code, Label: label}, original: info.original})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].original != rows[j].original {
			return rows[i].original
		}
		return strings.ToLower(rows[i].choice.Label) < strings.ToLower(rows[j].choice.Label)
	})
	choices := make([]Choice, len(rows))
	for i, row := range rows {
		choices[i] = row.choice
	}
	return choices
}

// VideoHasOwnAudio reports whether the chosen video format already includes audio.
func VideoHasOwnAudio(formats []Format, formatID string) bool {
	video, ok := findFormat(formats, formatID)
	return ok && video.HasAudio()
}

// PlanVideo builds a download plan for a chosen video format id.
// A single video uses that format id. A playlist uses a height and codec
// selector, because one video's format id is not stable across entries.
func PlanVideo(formats []Format, formatID, language string, playlist bool) (Plan, error) {
	video, ok := findFormat(formats, formatID)
	if !ok || !video.HasVideo() {
		return Plan{}, fmt.Errorf("video format %s not found", formatID)
	}

	var audio Format
	if !video.HasAudio() {
		audio, _ = pairAudio(video, formats, language)
	}

	size := video.Size()
	if !video.HasAudio() && audio.FormatID != "" {
		if size > 0 && audio.Size() > 0 {
			size += audio.Size()
		} else {
			size = 0
		}
	}

	return Plan{
		Label:         planVideoLabel(video, audio),
		EstimatedSize: size,
		Selector:      []string{"-f", videoSelector(video, audio, playlist)},
	}, nil
}

// PlanAudioOriginal downloads one audio stream without converting it.
func PlanAudioOriginal(formats []Format, formatID string, playlist bool) (Plan, error) {
	audio, ok := findFormat(formats, formatID)
	if !ok || !audio.HasAudio() {
		return Plan{}, fmt.Errorf("audio format %s not found", formatID)
	}
	return Plan{
		Label:         audioChoiceLabel(audio),
		EstimatedSize: audio.Size(),
		Selector:      []string{"-f", audioSelector(audio, playlist)},
	}, nil
}

// PlanAudioConvert extracts audio and encodes it with ffmpeg.
// quality is "0" for best, or a yt-dlp bitrate such as "128K".
func PlanAudioConvert(codec, quality, language string) (Plan, error) {
	switch codec {
	case "mp3", "m4a", "opus", "flac":
	default:
		return Plan{}, fmt.Errorf("unsupported audio format %s", codec)
	}
	if quality == "" {
		quality = "0"
	}

	label := strings.ToUpper(codec)
	switch quality {
	case "0":
		label += " · best"
	default:
		label += " · " + strings.TrimSuffix(quality, "K") + " kbps"
	}

	return Plan{
		Label: label,
		Selector: []string{
			"-f", audioStreamSelector(language),
			"-x",
			"--audio-format", codec,
			"--audio-quality", quality,
		},
	}, nil
}

// NeedsFFmpeg reports whether the plan merges streams or converts audio.
func (p Plan) NeedsFFmpeg() bool {
	for i, arg := range p.Selector {
		if arg == "-x" {
			return true
		}
		if arg == "-f" && i+1 < len(p.Selector) && strings.Contains(p.Selector[i+1], "+") {
			return true
		}
		if arg == "--embed-subs" || arg == "--convert-subs" {
			return true
		}
	}
	return false
}

// FormatBytes renders a byte count for prompts. An unknown size is empty.
func FormatBytes(n int64) string {
	if n <= 0 {
		return ""
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div := int64(unit)
	exp := 0
	for n/div >= unit && exp < 3 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

func videoCandidates(formats []Format) []Format {
	out := make([]Format, 0, len(formats))
	for _, f := range formats {
		if f.HasDRM || f.storyboard() || f.unsupportedContainer() || !f.HasVideo() || f.Height <= 0 {
			continue
		}
		out = append(out, f)
	}
	return out
}

func audioCandidatesFor(formats []Format, language string) []Format {
	all := audioCandidates(formats)
	if language == "" {
		return all
	}
	filtered := make([]Format, 0, len(all))
	for _, f := range all {
		if matchesLanguage(f, language) {
			filtered = append(filtered, f)
		}
	}
	if len(filtered) == 0 {
		return all
	}
	return filtered
}

func audioCandidates(formats []Format) []Format {
	out := make([]Format, 0, len(formats))
	drc := make([]Format, 0)
	for _, f := range formats {
		if f.HasDRM || f.storyboard() || !f.AudioOnly() {
			continue
		}
		// DRC is a compressed-dynamic-range copy of a normal track, not a
		// higher quality. Keep it only when nothing else exists.
		if isDRC(f) {
			drc = append(drc, f)
			continue
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		return drc
	}
	return out
}

func videoVariants(formats []Format, height int) []Format {
	groups := map[string]Format{}
	for _, f := range videoCandidates(formats) {
		if f.Height != height {
			continue
		}
		key := strings.ToLower(f.Ext) + "|" + videoCodecName(f.VCodec)
		prev, ok := groups[key]
		if !ok || videoBetter(f, prev) {
			groups[key] = f
		}
	}
	out := make([]Format, 0, len(groups))
	for _, f := range groups {
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := variantRank(out[i]), variantRank(out[j])
		if left != right {
			return left < right
		}
		return videoBetter(out[i], out[j])
	})
	return out
}

func (f Format) storyboard() bool {
	if strings.EqualFold(f.Ext, "mhtml") {
		return true
	}
	if strings.Contains(strings.ToLower(f.FormatNote), "storyboard") {
		return true
	}
	id := strings.ToLower(f.FormatID)
	if !strings.HasPrefix(id, "sb") || len(id) == 2 {
		return false
	}
	for _, r := range id[2:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (f Format) unsupportedContainer() bool {
	switch strings.ToLower(f.Ext) {
	case "jpg", "jpeg", "png", "webp", "gif", "mhtml":
		return true
	default:
		return false
	}
}

func pairAudio(video Format, formats []Format, language string) (Format, bool) {
	candidates := audioCandidates(formats)
	if language == "" {
		language = defaultAudioLanguage(candidates)
	}
	if language != "" {
		filtered := make([]Format, 0, len(candidates))
		for _, f := range candidates {
			if matchesLanguage(f, language) {
				filtered = append(filtered, f)
			}
		}
		if len(filtered) > 0 {
			candidates = filtered
		}
	}
	if len(candidates) == 0 {
		return Format{}, false
	}
	best := candidates[0]
	for _, f := range candidates[1:] {
		if audioBetter(video, f, best) {
			best = f
		}
	}
	return best, true
}

func videoBetter(a, b Format) bool {
	if ar, br := streamBitrate(a), streamBitrate(b); ar != br {
		return ar > br
	}
	if ar, br := protocolRank(a.Protocol), protocolRank(b.Protocol); ar != br {
		return ar < br
	}
	if a.Size() != b.Size() {
		return a.Size() > b.Size()
	}
	return a.FormatID < b.FormatID
}

func audioBetter(video, a, b Format) bool {
	if ar, br := audioExtRank(video.Ext, a.Ext), audioExtRank(video.Ext, b.Ext); ar != br {
		return ar < br
	}
	if isDRC(a) != isDRC(b) {
		return !isDRC(a)
	}
	if ar, br := bitrateKbps(a), bitrateKbps(b); ar != br {
		return ar > br
	}
	if ar, br := languageRank(a.Language), languageRank(b.Language); ar != br {
		return ar < br
	}
	if ar, br := protocolRank(a.Protocol), protocolRank(b.Protocol); ar != br {
		return ar < br
	}
	return a.FormatID < b.FormatID
}

func audioListBetter(a, b Format) bool {
	if isDRC(a) != isDRC(b) {
		return !isDRC(a)
	}
	if ar, br := bitrateKbps(a), bitrateKbps(b); ar != br {
		return ar > br
	}
	if ar, br := languageRank(a.Language), languageRank(b.Language); ar != br {
		return ar < br
	}
	if ar, br := protocolRank(a.Protocol), protocolRank(b.Protocol); ar != br {
		return ar < br
	}
	return a.FormatID < b.FormatID
}

func audioExtRank(videoExt, audioExt string) int {
	videoExt = strings.ToLower(videoExt)
	audioExt = strings.ToLower(audioExt)
	switch videoExt {
	case "mp4", "m4a", "mov":
		if audioExt == "m4a" || audioExt == "mp4" {
			return 0
		}
	case "webm":
		if audioExt == "webm" {
			return 0
		}
	case "mkv":
		if audioExt == "m4a" || audioExt == "webm" || audioExt == "mp4" {
			return 0
		}
	}
	return 1
}

func variantRank(f Format) int {
	ext := strings.ToLower(f.Ext)
	codec := videoCodecName(f.VCodec)
	switch {
	case ext == "mp4" && codec == "H.264":
		return 0
	case ext == "mp4" && codec == "HEVC":
		return 1
	case ext == "webm" && codec == "VP9":
		return 2
	case ext == "mp4" && codec == "AV1":
		return 3
	case ext == "webm" && codec == "AV1":
		return 4
	default:
		return 10
	}
}

func protocolRank(protocol string) int {
	p := strings.ToLower(protocol)
	switch {
	case p == "https" || p == "http":
		return 0
	case strings.Contains(p, "dash"):
		return 1
	case strings.Contains(p, "m3u8"), strings.Contains(p, "hls"):
		return 3
	default:
		return 2
	}
}

func languageRank(language string) int {
	if language == "" {
		return 0
	}
	return 1
}

func matchesLanguage(f Format, language string) bool {
	switch language {
	case "":
		return true
	case "default":
		return f.Language == ""
	default:
		return f.Language == language
	}
}

func defaultAudioLanguage(candidates []Format) string {
	seen := map[string]struct{}{}
	original := ""
	for _, f := range candidates {
		if f.Language == "" {
			continue
		}
		seen[f.Language] = struct{}{}
		if original == "" && isOriginalAudio(f) {
			original = f.Language
		}
	}
	if len(seen) > 1 {
		return original
	}
	return ""
}

func isOriginalAudio(f Format) bool {
	if strings.Contains(strings.ToLower(f.FormatNote), "original") {
		return true
	}
	return f.LanguagePreference > 0
}

func languageDisplay(f Format) string {
	name := cleanLanguageNote(f.FormatNote)
	if name == "" {
		name = f.Language
	}
	if name == "" {
		return ""
	}
	if isOriginalAudio(f) && !strings.Contains(strings.ToLower(name), "original") {
		return name + " (original)"
	}
	return name
}

func cleanLanguageNote(note string) string {
	note = strings.TrimSpace(note)
	if i := strings.IndexByte(note, ','); i >= 0 {
		note = strings.TrimSpace(note[:i])
	}
	note = strings.TrimSuffix(note, " (default)")
	note = strings.TrimSpace(note)
	for _, suffix := range []string{" (original)", " - dubbed", " - original", " original"} {
		if strings.HasSuffix(strings.ToLower(note), suffix) {
			note = strings.TrimSpace(note[:len(note)-len(suffix)])
		}
	}
	return strings.TrimSpace(note)
}

func preferLanguageName(current, next string) string {
	if next == "" {
		return current
	}
	if current == "" || (isASCII(next) && !isASCII(current)) {
		return next
	}
	return current
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

func audioStreamSelector(language string) string {
	if language == "" {
		return "bestaudio/best"
	}
	return "bestaudio[language=" + language + "]/best[language=" + language + "]/bestaudio/best"
}

func isDRC(f Format) bool {
	note := strings.ToLower(f.FormatNote)
	id := strings.ToLower(f.FormatID)
	return strings.Contains(note, "drc") || strings.Contains(id, "drc")
}

func streamBitrate(f Format) int {
	if f.TBR > 0 {
		return int(math.Round(f.TBR))
	}
	if f.VBR > 0 {
		return int(math.Round(f.VBR))
	}
	return bitrateKbps(f)
}

func bitrateKbps(f Format) int {
	v := f.ABR
	if v <= 0 {
		v = f.TBR
	}
	if v <= 0 {
		return 0
	}
	return int(math.Round(v))
}

// bucketBitrate snaps a measured bitrate onto a short conversion ladder.
// 106 kbps and 107 kbps are the same choice.
func bucketBitrate(kbps int) int {
	if kbps <= 0 {
		return 0
	}
	for _, step := range []int{320, 256, 192, 160, 128, 96, 64, 48, 32} {
		if kbps >= step {
			return step
		}
	}
	return kbps
}

func videoCodecName(vcodec string) string {
	v := strings.ToLower(vcodec)
	switch {
	case v == "" || v == "none":
		return ""
	case strings.HasPrefix(v, "avc1"), strings.HasPrefix(v, "h264"):
		return "H.264"
	case strings.HasPrefix(v, "av01"), strings.Contains(v, "av1"):
		return "AV1"
	case strings.HasPrefix(v, "vp09"), strings.HasPrefix(v, "vp9"):
		return "VP9"
	case strings.HasPrefix(v, "vp8"):
		return "VP8"
	case strings.HasPrefix(v, "hev1"), strings.HasPrefix(v, "hvc1"), strings.Contains(v, "hevc"):
		return "HEVC"
	default:
		if i := strings.IndexByte(vcodec, '.'); i > 0 {
			return vcodec[:i]
		}
		return vcodec
	}
}

func audioCodecName(acodec string) string {
	a := strings.ToLower(acodec)
	switch {
	case a == "" || a == "none":
		return ""
	case strings.HasPrefix(a, "mp4a"), strings.Contains(a, "aac"):
		return "AAC"
	case strings.Contains(a, "opus"):
		return "Opus"
	case strings.Contains(a, "flac"):
		return "FLAC"
	case strings.HasPrefix(a, "mp3"), strings.Contains(a, "mp3"):
		return "MP3"
	case strings.Contains(a, "vorbis"):
		return "Vorbis"
	default:
		if i := strings.IndexByte(acodec, '.'); i > 0 {
			return acodec[:i]
		}
		return acodec
	}
}

func videoCodecPrefix(vcodec string) string {
	v := strings.ToLower(vcodec)
	switch {
	case strings.HasPrefix(v, "avc1"), strings.HasPrefix(v, "h264"):
		return "avc1"
	case strings.HasPrefix(v, "vp09"), strings.HasPrefix(v, "vp9"):
		return "vp9"
	case strings.HasPrefix(v, "av01"):
		return "av01"
	case strings.HasPrefix(v, "hev1"), strings.HasPrefix(v, "hvc1"):
		return "hev1"
	case v == "" || v == "none":
		return ""
	default:
		if i := strings.IndexByte(v, '.'); i > 0 {
			return v[:i]
		}
		return v
	}
}

func audioCodecPrefix(acodec string) string {
	a := strings.ToLower(acodec)
	switch {
	case strings.HasPrefix(a, "mp4a"):
		return "mp4a"
	case strings.Contains(a, "opus"):
		return "opus"
	case strings.Contains(a, "vorbis"):
		return "vorbis"
	case strings.HasPrefix(a, "mp3"):
		return "mp3"
	case a == "" || a == "none":
		return ""
	default:
		if i := strings.IndexByte(a, '.'); i > 0 {
			return a[:i]
		}
		return a
	}
}

func dynamicRangeLabel(dynamicRange string) string {
	if dynamicRange == "" || strings.EqualFold(dynamicRange, "SDR") {
		return ""
	}
	return dynamicRange
}

func videoVariantLabel(f Format) string {
	parts := []string{strings.ToUpper(f.Ext)}
	if codec := videoCodecName(f.VCodec); codec != "" {
		parts = append(parts, codec)
	}
	if dr := dynamicRangeLabel(f.DynamicRange); dr != "" {
		parts = append(parts, dr)
	}
	if size := FormatBytes(f.Size()); size != "" {
		parts = append(parts, "~"+size)
	}
	return strings.Join(parts, " · ")
}

func planVideoLabel(video, audio Format) string {
	label := fmt.Sprintf("%dp · %s", video.Height, videoVariantLabel(video))
	if video.HasAudio() || audio.FormatID == "" {
		return label
	}
	if spoken := languageDisplay(audio); spoken != "" {
		return label + " + " + spoken + " audio"
	}
	name := audioCodecName(audio.ACodec)
	if name == "" {
		name = strings.ToUpper(audio.Ext)
	}
	return label + " + " + name + " audio"
}

func audioChoiceLabel(f Format) string {
	var parts []string
	if kbps := bitrateKbps(f); kbps > 0 {
		parts = append(parts, fmt.Sprintf("%d kbps", kbps))
	}
	if name := audioCodecName(f.ACodec); name != "" {
		parts = append(parts, name)
	}
	if f.Ext != "" {
		parts = append(parts, strings.ToUpper(f.Ext))
	}
	if spoken := languageDisplay(f); spoken != "" {
		parts = append(parts, spoken)
	}
	if isDRC(f) {
		parts = append(parts, "DRC")
	}
	if size := FormatBytes(f.Size()); size != "" {
		parts = append(parts, "~"+size)
	}
	if len(parts) == 0 {
		return f.FormatID
	}
	return strings.Join(parts, " · ")
}

func videoSelector(video, audio Format, playlist bool) string {
	if !playlist {
		if video.HasAudio() || audio.FormatID == "" {
			return video.FormatID
		}
		return video.FormatID + "+" + audio.FormatID
	}

	filters := []string{
		"height=" + strconv.Itoa(video.Height),
		"ext=" + strings.ToLower(video.Ext),
	}
	if codec := videoCodecPrefix(video.VCodec); codec != "" {
		filters = append(filters, "vcodec^="+codec)
	}
	filter := "[" + strings.Join(filters, "][") + "]"
	height := strconv.Itoa(video.Height)
	ext := strings.ToLower(video.Ext)

	if video.HasAudio() {
		return "b" + filter + "/bv*[height=" + height + "][ext=" + ext + "]+ba"
	}

	selector := "bv*" + filter + "+" + baFilter(audio)
	return selector + "/b[height=" + height + "]"
}

func baFilter(audio Format) string {
	var filters []string
	if audio.Ext != "" {
		filters = append(filters, "ext="+strings.ToLower(audio.Ext))
	}
	if audio.Language != "" {
		filters = append(filters, "language="+audio.Language)
	}
	if len(filters) == 0 {
		return "ba"
	}
	return "ba[" + strings.Join(filters, "][") + "]"
}

func audioSelector(audio Format, playlist bool) string {
	if !playlist {
		return audio.FormatID
	}
	var filters []string
	if audio.Ext != "" {
		filters = append(filters, "ext="+strings.ToLower(audio.Ext))
	}
	if audio.Language != "" {
		filters = append(filters, "language="+audio.Language)
	}
	if prefix := audioCodecPrefix(audio.ACodec); prefix != "" {
		filters = append(filters, "acodec^="+prefix)
	}
	if kbps := bitrateKbps(audio); kbps > 0 {
		filters = append(filters, fmt.Sprintf("abr<=%d", kbps))
	}
	if len(filters) == 0 {
		return "ba/best"
	}
	return "ba[" + strings.Join(filters, "][") + "]/ba"
}

func findFormat(formats []Format, id string) (Format, bool) {
	var matches []Format
	for _, f := range formats {
		if f.FormatID == id {
			matches = append(matches, f)
		}
	}
	if len(matches) == 0 {
		return Format{}, false
	}
	best := matches[0]
	for _, f := range matches[1:] {
		switch {
		case f.HasVideo() && (!best.HasVideo() || videoBetter(f, best)):
			best = f
		case !f.HasVideo() && !best.HasVideo() && bitrateKbps(f) > bitrateKbps(best):
			best = f
		}
	}
	return best, true
}
