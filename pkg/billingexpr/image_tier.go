package billingexpr

import (
	"fmt"
	"strconv"
	"strings"
)

const image2KMaxPixels = 2560 * 1440

// ImageTier maps OpenAI-compatible image request size/quality parameters to
// concrete billing tiers. The "auto" quality is not a billing tier: it falls
// through to resolution-based classification.
func ImageTier(size interface{}, quality interface{}) string {
	q := strings.ToLower(strings.TrimSpace(fmt.Sprint(quality)))
	switch q {
	case "high", "4k", "ultra":
		return "4k"
	case "medium", "2k", "hd":
		return "2k"
	case "low", "1k", "standard":
		return "1k"
	}
	return imageTierFromSize(fmt.Sprint(size))
}

func imageTierFromSize(size string) string {
	normalized := strings.ToLower(strings.TrimSpace(size))
	switch normalized {
	case "", "<nil>", "auto":
		return "2k"
	case "1024x1024", "1280x720", "720x1280":
		return "1k"
	case "1536x1024", "1024x1536", "1792x1024", "1024x1792", "2048x2048", "2048x1152", "1152x2048", "2560x1440", "1440x2560", "1024x1824":
		return "2k"
	case "3840x2160", "2160x3840":
		return "4k"
	}

	width, height, ok := parseImageSizeDimensions(normalized)
	if !ok {
		return "2k"
	}
	if height > 0 && width > image2KMaxPixels/height {
		return "4k"
	}
	if width <= 1280 && height <= 1280 && width*height <= 1280*720 {
		return "1k"
	}
	return "2k"
}

func parseImageSizeDimensions(size string) (int, int, bool) {
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || width <= 0 {
		return 0, 0, false
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}
