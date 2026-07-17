package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	codexImagePromptMinPixels = 655_360
	codexImagePromptMaxPixels = 8_294_400
	codexImagePromptMaxEdge   = 3840
)

var (
	codexImageDimensionsPattern = regexp.MustCompile(`(?i)(\d{3,4})\s*(?:x|×|\*)\s*(\d{3,4})`)
	codexImageRatioPattern      = regexp.MustCompile(`(?i)(比例|宽高比|aspect\s*ratio)?\s*(\d{1,2})\s*[:：]\s*(\d{1,2})`)
	codexImageFormatPattern     = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(jpe?g|png|webp)(?:$|[^a-z0-9])`)
	codexImageCountPattern      = regexp.MustCompile(`(?i)(?:生成|制作|创建|给我|输出|出|generate|create|make|give\s+me)\s*(\d{1,2})\s*(?:张|幅|个版本|个方案|images?|versions?|variations?)`)
	codexImageChineseCount      = regexp.MustCompile(`(?:生成|制作|创建|给我|输出|出)\s*([一二三四五六七八九十两]+)\s*(?:张|幅|个版本|个方案)`)
	codexImageCompression       = regexp.MustCompile(`(?i)(?:压缩质量|压缩率|jpe?g\s*quality|webp\s*quality)\s*[:=]?\s*(\d{1,3})\s*%?`)
	codexImageDPI               = regexp.MustCompile(`(?i)\d+\s*dpi\b`)
	codexImageColorSpace        = regexp.MustCompile(`(?i)\b(?:cmyk|display\s*p3|adobe\s*rgb)\b`)
	codexImageTargetFileSize    = regexp.MustCompile(`(?i)(?:小于|不超过|控制在|under|below|less\s+than)\s*\d+(?:\.\d+)?\s*(?:kb|mb|gb|k|m)\b`)
)

// ApplyCodexImagePromptControls promotes explicit output requirements from the
// prompt into Images API fields that stock Codex Desktop currently fixes to
// auto or omits. Callers must gate this to the official channel-managed Codex
// image bridge; ordinary Images API requests keep their explicit contract.
func ApplyCodexImagePromptControls(body []byte, req *OpenAIImagesRequest) ([]byte, []string, error) {
	if req == nil || req.Multipart || len(body) == 0 || !gjson.ValidBytes(body) {
		return body, nil, nil
	}
	prompt := strings.TrimSpace(req.Prompt)
	if err := validateUnsupportedCodexPromptControls(prompt, body); err != nil {
		return nil, nil, err
	}
	if prompt == "" {
		return body, nil, nil
	}

	rewritten := body
	applied := make([]string, 0, 4)
	var err error

	if shouldInferCodexPromptField(body, "size", "auto") {
		inferredSize, inferErr := inferCodexPromptSize(prompt)
		if inferErr != nil {
			return nil, nil, inferErr
		}
		if inferredSize != "" {
			rewritten, err = sjson.SetBytes(rewritten, "size", inferredSize)
			if err != nil {
				return nil, nil, fmt.Errorf("apply Codex prompt image size: %w", err)
			}
			req.Size = inferredSize
			req.ExplicitSize = true
			applied = append(applied, "size")
		}
	}

	if shouldInferCodexPromptField(body, "quality", "auto") {
		quality, inferErr := inferCodexPromptQuality(prompt)
		if inferErr != nil {
			return nil, nil, inferErr
		}
		if quality != "" {
			rewritten, err = sjson.SetBytes(rewritten, "quality", quality)
			if err != nil {
				return nil, nil, fmt.Errorf("apply Codex prompt image quality: %w", err)
			}
			req.Quality = quality
			applied = append(applied, "quality")
		}
	}

	if shouldInferCodexPromptField(body, "background", "auto") {
		background, inferErr := inferCodexPromptBackground(prompt)
		if inferErr != nil {
			return nil, nil, inferErr
		}
		if background != "" {
			rewritten, err = sjson.SetBytes(rewritten, "background", background)
			if err != nil {
				return nil, nil, fmt.Errorf("apply Codex prompt image background: %w", err)
			}
			req.Background = background
			applied = append(applied, "background")
		}
	}

	if shouldInferCodexPromptField(body, "output_format") && promptRequestsPNG(prompt) {
		rewritten, err = sjson.SetBytes(rewritten, "output_format", "png")
		if err != nil {
			return nil, nil, fmt.Errorf("apply Codex prompt image output format: %w", err)
		}
		req.OutputFormat = "png"
		applied = append(applied, "output_format")
	}

	if len(applied) == 0 {
		return body, nil, nil
	}
	req.Body = rewritten
	sum := sha256.Sum256(rewritten)
	req.bodyHash = hex.EncodeToString(sum[:8])
	req.SizeTier = normalizeOpenAIImageSizeTier(req.Size)
	req.HasNativeOptions = hasOpenAINativeImageOptions(func(path string) bool {
		return gjson.GetBytes(rewritten, path).Exists()
	})
	req.RequiredCapability = classifyOpenAIImagesCapability(req)
	sort.Strings(applied)
	return rewritten, applied, nil
}

func shouldInferCodexPromptField(body []byte, path string, autoValues ...string) bool {
	value := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, path).String()))
	if value == "" {
		return true
	}
	for _, autoValue := range autoValues {
		if value == strings.ToLower(strings.TrimSpace(autoValue)) {
			return true
		}
	}
	return false
}

func validateUnsupportedCodexPromptControls(prompt string, body []byte) error {
	explicitSize := strings.TrimSpace(gjson.GetBytes(body, "size").String())
	if explicitSize != "" && !strings.EqualFold(explicitSize, "auto") {
		width, height, ok := parseCodexPromptSize(explicitSize)
		if !ok {
			return fmt.Errorf("Codex Desktop image size must use WIDTHxHEIGHT or auto")
		}
		if err := validateCodexPromptDimensions(width, height); err != nil {
			return err
		}
	}
	outputFormatResult := gjson.GetBytes(body, "output_format")
	explicitFormat := strings.ToLower(strings.TrimSpace(outputFormatResult.String()))
	if explicitFormat == "jpg" {
		explicitFormat = "jpeg"
	}
	if explicitFormat != "" && explicitFormat != "png" {
		return fmt.Errorf("Codex Desktop only saves generated artifacts as PNG; requested output format %s is not supported by the stock client", explicitFormat)
	}
	if !outputFormatResult.Exists() || explicitFormat == "" {
		formats := uniqueCodexPromptFormats(prompt)
		if len(formats) > 1 {
			return fmt.Errorf("Codex image prompt contains conflicting output formats: %s", strings.Join(formats, ", "))
		}
		if len(formats) == 1 && formats[0] != "png" {
			return fmt.Errorf("Codex Desktop only saves generated artifacts as PNG; requested output format %s is not supported by the stock client", formats[0])
		}
	}

	if explicitCount := int(gjson.GetBytes(body, "n").Int()); explicitCount > 1 {
		return fmt.Errorf("Codex Desktop consumes one image per imagegen call; requesting %d images in one call would create unused billable outputs", explicitCount)
	}
	if !gjson.GetBytes(body, "n").Exists() {
		count, err := inferCodexPromptOutputCount(prompt)
		if err != nil {
			return err
		}
		if count > 1 {
			return fmt.Errorf("Codex Desktop consumes one image per imagegen call; requesting %d images in one call would create unused billable outputs", count)
		}
	}

	if gjson.GetBytes(body, "output_compression").Exists() || codexImageCompression.MatchString(prompt) {
		return fmt.Errorf("Codex Desktop PNG artifacts do not support requested output compression")
	}
	if codexImageDPI.MatchString(prompt) {
		return fmt.Errorf("Codex Desktop image bridge cannot apply requested DPI metadata")
	}
	if codexImageColorSpace.MatchString(prompt) {
		return fmt.Errorf("Codex Desktop image bridge cannot apply requested color-space conversion")
	}
	if codexImageTargetFileSize.MatchString(prompt) {
		return fmt.Errorf("Codex Desktop image bridge cannot guarantee requested target file size")
	}
	return nil
}

func inferCodexPromptSize(prompt string) (string, error) {
	dimensions := make(map[string][2]int)
	for _, match := range codexImageDimensionsPattern.FindAllStringSubmatch(prompt, -1) {
		width, _ := strconv.Atoi(match[1])
		height, _ := strconv.Atoi(match[2])
		size := fmt.Sprintf("%dx%d", width, height)
		dimensions[size] = [2]int{width, height}
	}
	if len(dimensions) > 1 {
		return "", fmt.Errorf("Codex image prompt contains conflicting explicit image dimensions")
	}

	var explicit [2]int
	for _, value := range dimensions {
		explicit = value
	}
	if explicit[0] > 0 {
		if err := validateCodexPromptDimensions(explicit[0], explicit[1]); err != nil {
			return "", err
		}
	}

	ratio, err := inferCodexPromptRatio(prompt)
	if err != nil {
		return "", err
	}
	orientation, err := inferCodexPromptOrientation(prompt)
	if err != nil {
		return "", err
	}
	if explicit[0] > 0 {
		if ratio[0] > 0 && explicit[0]*ratio[1] != explicit[1]*ratio[0] {
			return "", fmt.Errorf("explicit image dimensions conflict with aspect ratio")
		}
		if orientation != "" && !dimensionsMatchOrientation(explicit[0], explicit[1], orientation) {
			return "", fmt.Errorf("explicit image dimensions conflict with requested orientation")
		}
		return fmt.Sprintf("%dx%d", explicit[0], explicit[1]), nil
	}

	if ratio[0] > 0 {
		if orientation != "" && !dimensionsMatchOrientation(ratio[0], ratio[1], orientation) {
			return "", fmt.Errorf("aspect ratio conflicts with requested orientation")
		}
		width, height := codexPromptSizeForRatio(ratio[0], ratio[1])
		if err := validateCodexPromptDimensions(width, height); err != nil {
			return "", err
		}
		return fmt.Sprintf("%dx%d", width, height), nil
	}

	switch orientation {
	case "landscape":
		return "2048x1152", nil
	case "portrait":
		return "1152x2048", nil
	case "square":
		return "2048x2048", nil
	default:
		return "", nil
	}
}

func validateCodexPromptDimensions(width, height int) error {
	if width <= 0 || height <= 0 || width > codexImagePromptMaxEdge || height > codexImagePromptMaxEdge {
		return fmt.Errorf("requested image dimensions must be between 1 and %d pixels per edge", codexImagePromptMaxEdge)
	}
	if width%16 != 0 || height%16 != 0 {
		return fmt.Errorf("requested image dimensions must be multiples of 16")
	}
	pixels := width * height
	if pixels < codexImagePromptMinPixels || pixels > codexImagePromptMaxPixels {
		return fmt.Errorf("requested image dimensions must contain between %d and %d pixels", codexImagePromptMinPixels, codexImagePromptMaxPixels)
	}
	longEdge, shortEdge := width, height
	if height > width {
		longEdge, shortEdge = height, width
	}
	if longEdge > shortEdge*3 {
		return fmt.Errorf("requested image aspect ratio must not exceed 3:1")
	}
	return nil
}

func inferCodexPromptRatio(prompt string) ([2]int, error) {
	var ratio [2]int
	for _, match := range codexImageRatioPattern.FindAllStringSubmatch(prompt, -1) {
		label := strings.TrimSpace(match[1])
		width, _ := strconv.Atoi(match[2])
		height, _ := strconv.Atoi(match[3])
		if width <= 0 || height <= 0 {
			continue
		}
		divisor := greatestCommonDivisor(width, height)
		candidate := [2]int{width / divisor, height / divisor}
		if label == "" && !isCommonCodexPromptRatio(candidate) {
			continue
		}
		if ratio[0] != 0 && ratio != candidate {
			return [2]int{}, fmt.Errorf("Codex image prompt contains conflicting aspect ratios")
		}
		ratio = candidate
	}
	if ratio[0] > 0 {
		longEdge, shortEdge := ratio[0], ratio[1]
		if shortEdge > longEdge {
			longEdge, shortEdge = shortEdge, longEdge
		}
		if longEdge > shortEdge*3 {
			return [2]int{}, fmt.Errorf("requested image aspect ratio must not exceed 3:1")
		}
	}
	return ratio, nil
}

func isCommonCodexPromptRatio(ratio [2]int) bool {
	for _, common := range [][2]int{{1, 1}, {3, 2}, {2, 3}, {4, 3}, {3, 4}, {16, 9}, {9, 16}, {21, 9}, {9, 21}} {
		if ratio == common {
			return true
		}
	}
	return false
}

func inferCodexPromptOrientation(prompt string) (string, error) {
	lower := strings.ToLower(prompt)
	orientations := make(map[string]struct{})
	for orientation, terms := range map[string][]string{
		"landscape": {"横版", "横向", "landscape orientation", "landscape format"},
		"portrait":  {"竖版", "纵向", "portrait orientation", "portrait format"},
		"square":    {"方形", "正方形", "square image", "square format"},
	} {
		for _, term := range terms {
			if strings.Contains(lower, term) {
				orientations[orientation] = struct{}{}
				break
			}
		}
	}
	if len(orientations) > 1 {
		return "", fmt.Errorf("Codex image prompt contains conflicting orientations")
	}
	for orientation := range orientations {
		return orientation, nil
	}
	return "", nil
}

func codexPromptSizeForRatio(widthRatio, heightRatio int) (int, int) {
	longRatio := widthRatio
	if heightRatio > longRatio {
		longRatio = heightRatio
	}
	scale := 2048 / (16 * longRatio)
	if scale < 1 {
		scale = 1
	}
	return widthRatio * 16 * scale, heightRatio * 16 * scale
}

func dimensionsMatchOrientation(width, height int, orientation string) bool {
	switch orientation {
	case "landscape":
		return width > height
	case "portrait":
		return height > width
	case "square":
		return width == height
	default:
		return true
	}
}

func greatestCommonDivisor(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func inferCodexPromptQuality(prompt string) (string, error) {
	lower := strings.ToLower(prompt)
	values := make(map[string]struct{})
	for quality, terms := range map[string][]string{
		"low":    {"低质量", "草稿质量", "快速草稿", "low quality", "quality=low", "quality: low"},
		"medium": {"中等质量", "标准质量", "medium quality", "quality=medium", "quality: medium"},
		"high":   {"高质量", "最高质量", "最终稿质量", "high quality", "quality=high", "quality: high"},
	} {
		for _, term := range terms {
			if strings.Contains(lower, term) {
				values[quality] = struct{}{}
				break
			}
		}
	}
	if len(values) > 1 {
		return "", fmt.Errorf("Codex image prompt contains conflicting quality levels")
	}
	for quality := range values {
		return quality, nil
	}
	return "", nil
}

func inferCodexPromptBackground(prompt string) (string, error) {
	lower := strings.ToLower(prompt)
	opaqueTerms := []string{"不透明背景", "无透明通道", "opaque background", "non-transparent background", "no alpha channel"}
	opaque := codexPromptContainsAny(lower, opaqueTerms...)
	transparentText := lower
	for _, term := range opaqueTerms {
		transparentText = strings.ReplaceAll(transparentText, term, "")
	}
	transparent := codexPromptContainsAny(transparentText, "透明背景", "背景透明", "透明通道", "alpha 通道", "alpha channel", "transparent background")
	if transparent && opaque {
		return "", fmt.Errorf("Codex image prompt contains conflicting background transparency requirements")
	}
	if transparent {
		return "transparent", nil
	}
	if opaque {
		return "opaque", nil
	}
	return "", nil
}

func uniqueCodexPromptFormats(prompt string) []string {
	values := make(map[string]struct{})
	for _, match := range codexImageFormatPattern.FindAllStringSubmatch(prompt, -1) {
		format := strings.ToLower(strings.TrimSpace(match[1]))
		if format == "jpg" {
			format = "jpeg"
		}
		values[format] = struct{}{}
	}
	formats := make([]string, 0, len(values))
	for format := range values {
		formats = append(formats, format)
	}
	sort.Strings(formats)
	return formats
}

func promptRequestsPNG(prompt string) bool {
	formats := uniqueCodexPromptFormats(prompt)
	return len(formats) == 1 && formats[0] == "png"
}

func inferCodexPromptOutputCount(prompt string) (int, error) {
	counts := make(map[int]struct{})
	for _, match := range codexImageCountPattern.FindAllStringSubmatch(prompt, -1) {
		count, _ := strconv.Atoi(match[1])
		if count > 0 {
			counts[count] = struct{}{}
		}
	}
	for _, match := range codexImageChineseCount.FindAllStringSubmatch(prompt, -1) {
		if count := parseChineseImageCount(match[1]); count > 0 {
			counts[count] = struct{}{}
		}
	}
	if len(counts) > 1 {
		return 0, fmt.Errorf("Codex image prompt contains conflicting output counts")
	}
	for count := range counts {
		if count > 10 {
			return 0, fmt.Errorf("requested image count must not exceed 10")
		}
		return count, nil
	}
	return 0, nil
}

func parseChineseImageCount(value string) int {
	if value == "十" {
		return 10
	}
	if strings.HasPrefix(value, "十") {
		return 10 + chineseDigit(strings.TrimPrefix(value, "十"))
	}
	if strings.Contains(value, "十") {
		parts := strings.SplitN(value, "十", 2)
		return chineseDigit(parts[0])*10 + chineseDigit(parts[1])
	}
	return chineseDigit(value)
}

func chineseDigit(value string) int {
	switch value {
	case "一":
		return 1
	case "二", "两":
		return 2
	case "三":
		return 3
	case "四":
		return 4
	case "五":
		return 5
	case "六":
		return 6
	case "七":
		return 7
	case "八":
		return 8
	case "九":
		return 9
	default:
		return 0
	}
}

func codexPromptContainsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func normalizeCodexImageOutputBase64(encoded string, targetSize string) (string, string, error) {
	targetSize = strings.TrimSpace(targetSize)
	if targetSize == "" || strings.EqualFold(targetSize, "auto") {
		return encoded, "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", fmt.Errorf("decode generated image for exact-size validation: %w", err)
	}
	normalized, actualSize, err := normalizeCodexImageOutputBytes(raw, targetSize)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(normalized), actualSize, nil
}

func normalizeCodexImageOutputBytes(raw []byte, targetSize string) ([]byte, string, error) {
	targetWidth, targetHeight, ok := parseCodexPromptSize(targetSize)
	if !ok {
		return raw, "", nil
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, "", fmt.Errorf("decode generated image dimensions: %w", err)
	}
	actualSize := fmt.Sprintf("%dx%d", config.Width, config.Height)
	if config.Width == targetWidth && config.Height == targetHeight && format == "png" {
		return raw, actualSize, nil
	}

	source, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", fmt.Errorf("decode generated image for exact-size correction: %w", err)
	}
	target := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	xdraw.CatmullRom.Scale(target, target.Bounds(), source, source.Bounds(), xdraw.Over, nil)
	var output bytes.Buffer
	if err := png.Encode(&output, target); err != nil {
		return nil, "", fmt.Errorf("encode exact-size Codex PNG: %w", err)
	}
	if output.Len() > openAIImageMaxDownloadBytes {
		return nil, "", fmt.Errorf("exact-size Codex PNG exceeds %d bytes", openAIImageMaxDownloadBytes)
	}
	return output.Bytes(), fmt.Sprintf("%dx%d", targetWidth, targetHeight), nil
}

func normalizeCodexResponsesImageResults(results []openAIResponsesImageResult, targetSize string) error {
	if strings.TrimSpace(targetSize) == "" || strings.EqualFold(strings.TrimSpace(targetSize), "auto") {
		return nil
	}
	for index := range results {
		encoded, actualSize, err := normalizeCodexImageOutputBase64(results[index].Result, targetSize)
		if err != nil {
			return fmt.Errorf("normalize Codex image result %d: %w", index, err)
		}
		results[index].Result = encoded
		results[index].Size = actualSize
		results[index].OutputFormat = "png"
	}
	return nil
}

func parseCodexPromptSize(size string) (int, int, bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, heightErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}
