package dto

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type ImageRequest struct {
	Model             string          `json:"model"`
	Prompt            string          `json:"prompt" binding:"required"`
	N                 *uint           `json:"n,omitempty"`
	Size              string          `json:"size,omitempty"`
	Quality           string          `json:"quality,omitempty"`
	ResponseFormat    string          `json:"response_format,omitempty"`
	Async             *bool           `json:"async,omitempty"`
	Style             json.RawMessage `json:"style,omitempty"`
	User              json.RawMessage `json:"user,omitempty"`
	ExtraFields       json.RawMessage `json:"extra_fields,omitempty"`
	Background        json.RawMessage `json:"background,omitempty"`
	Moderation        json.RawMessage `json:"moderation,omitempty"`
	OutputFormat      json.RawMessage `json:"output_format,omitempty"`
	OutputCompression json.RawMessage `json:"output_compression,omitempty"`
	PartialImages     json.RawMessage `json:"partial_images,omitempty"`
	// Stream            bool            `json:"stream,omitempty"`
	Images        json.RawMessage `json:"images,omitempty"`
	Mask          json.RawMessage `json:"mask,omitempty"`
	InputFidelity json.RawMessage `json:"input_fidelity,omitempty"`
	Watermark     *bool           `json:"watermark,omitempty"`
	// zhipu 4v
	WatermarkEnabled json.RawMessage `json:"watermark_enabled,omitempty"`
	UserId           json.RawMessage `json:"user_id,omitempty"`
	Image            json.RawMessage `json:"image,omitempty"`
	// 用匿名参数接收额外参数
	Extra map[string]json.RawMessage `json:"-"`
}

func NormalizeOpenAIImageGenerationQuality(quality string) string {
	normalized := strings.ToLower(strings.TrimSpace(quality))
	switch normalized {
	case "auto", "low", "medium", "high":
		return normalized
	default:
		return "auto"
	}
}

func (i *ImageRequest) NormalizeOpenAIImageGenerationQuality() {
	if i == nil {
		return
	}
	i.Quality = NormalizeOpenAIImageGenerationQuality(i.Quality)
}

func MapOpenAIImageQualityToGPT2APITier(quality string) string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "low", "1k":
		return "1K"
	case "medium", "2k", "hd":
		return "2K"
	case "high", "4k", "ultra":
		return "4K"
	default:
		return quality
	}
}

func (i *ImageRequest) MapOpenAIImageQualityToGPT2APITier() {
	if i == nil {
		return
	}
	i.Quality = MapOpenAIImageQualityToGPT2APITier(i.Quality)
}

func MapGPT2APIImageSize(model string, quality string, size string) string {
	table := gpt2apiImageSizeTableForModel(model)
	if table == nil {
		return size
	}
	tier := strings.ToLower(strings.TrimSpace(MapOpenAIImageQualityToGPT2APITier(quality)))
	if tier == "" || tier == "auto" {
		return size
	}
	tierTable := table[tier]
	if tierTable == nil {
		return size
	}
	ratio, ok := parseImageAspectRatio(size)
	if !ok {
		return size
	}
	bestKey := ""
	bestDelta := math.MaxFloat64
	for key := range tierTable {
		keyRatio, ok := parseAspectRatioKey(key)
		if !ok {
			continue
		}
		delta := math.Abs(math.Log(ratio / keyRatio))
		if delta < bestDelta {
			bestDelta = delta
			bestKey = key
		}
	}
	if bestKey == "" {
		return size
	}
	return tierTable[bestKey]
}

func (i *ImageRequest) MapGPT2APIImageSize(model string) {
	if i == nil {
		return
	}
	i.Size = MapGPT2APIImageSize(model, i.Quality, i.Size)
}

func gpt2apiImageSizeTableForModel(model string) map[string]map[string]string {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "nano-banana-pro", "nano-banana-v2", "nano-banana":
		return map[string]map[string]string{
			"1k": {"1:1": "1024x1024", "3:2": "1264x848", "2:3": "848x1264", "4:3": "1152x864", "3:4": "864x1152", "5:4": "1152x928", "4:5": "928x1152", "16:9": "1376x768", "9:16": "768x1376", "21:9": "1584x672"},
			"2k": {"1:1": "2048x2048", "3:2": "2528x1696", "2:3": "1696x2528", "4:3": "2048x1536", "3:4": "1536x2048", "5:4": "2304x1856", "4:5": "1856x2304", "16:9": "2752x1536", "9:16": "1536x2752", "21:9": "3168x1344"},
			"4k": {"1:1": "4096x4096", "3:2": "5056x3392", "2:3": "3392x5056", "4:3": "4784x3584", "3:4": "3584x4784", "5:4": "4608x3712", "4:5": "3712x4608", "16:9": "5504x3072", "9:16": "3072x5504", "21:9": "6336x2688"},
		}
	case "gpt-image-2":
		return map[string]map[string]string{
			"1k": {"1:1": "1024x1024", "3:2": "1536x1024", "2:3": "1024x1536", "4:3": "1152x864", "3:4": "864x1152", "5:4": "1120x896", "4:5": "896x1120", "16:9": "1280x720", "9:16": "720x1280", "21:9": "1456x624"},
			"2k": {"1:1": "2048x2048", "3:2": "2496x1664", "2:3": "1664x2496", "4:3": "2304x1728", "3:4": "1728x2304", "5:4": "2240x1792", "4:5": "1792x2240", "16:9": "2560x1440", "9:16": "1440x2560", "21:9": "3024x1296"},
			"4k": {"1:1": "2480x2480", "3:2": "3056x2032", "2:3": "2032x3056", "4:3": "2880x2160", "3:4": "2160x2880", "5:4": "2784x2224", "4:5": "2224x2784", "16:9": "3328x1872", "9:16": "1872x3328", "21:9": "3808x1632"},
		}
	default:
		return nil
	}
}

func parseImageAspectRatio(size string) (float64, bool) {
	value := strings.ToLower(strings.TrimSpace(size))
	if value == "" {
		return 0, false
	}
	if strings.Contains(value, "x") {
		parts := strings.Split(value, "x")
		if len(parts) != 2 {
			return 0, false
		}
		w, errW := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		h, errH := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errW != nil || errH != nil || w <= 0 || h <= 0 {
			return 0, false
		}
		return w / h, true
	}
	return parseAspectRatioKey(value)
}

func parseAspectRatioKey(key string) (float64, bool) {
	parts := strings.Split(strings.TrimSpace(key), ":")
	if len(parts) != 2 {
		return 0, false
	}
	w, errW := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	h, errH := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, false
	}
	return w / h, true
}

func GPT2APIImageSizeMismatchError(model string, size string) string {
	model = strings.TrimSpace(model)
	if gpt2apiImageSizeTableForModel(model) == nil {
		return ""
	}
	return fmt.Sprintf("size %s is not a documented GPT2API size for %s", size, model)
}

func (i *ImageRequest) UnmarshalJSON(data []byte) error {
	// 先解析成 map[string]interface{}
	var rawMap map[string]json.RawMessage
	if err := common.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// 用 struct tag 获取所有已定义字段名
	knownFields := GetJSONFieldNames(reflect.TypeOf(*i))

	// 再正常解析已定义字段
	type Alias ImageRequest
	var known Alias
	if err := common.Unmarshal(data, &known); err != nil {
		return err
	}
	*i = ImageRequest(known)

	// 提取多余字段
	i.Extra = make(map[string]json.RawMessage)
	for k, v := range rawMap {
		if _, ok := knownFields[k]; !ok {
			i.Extra[k] = v
		}
	}
	return nil
}

// 序列化时需要重新把字段平铺
func (r ImageRequest) MarshalJSON() ([]byte, error) {
	// 将已定义字段转为 map
	type Alias ImageRequest
	alias := Alias(r)
	base, err := common.Marshal(alias)
	if err != nil {
		return nil, err
	}

	var baseMap map[string]json.RawMessage
	if err := common.Unmarshal(base, &baseMap); err != nil {
		return nil, err
	}

	// 不能合并ExtraFields！！！！！！！！
	// 合并 ExtraFields
	//for k, v := range r.Extra {
	//	if _, exists := baseMap[k]; !exists {
	//		baseMap[k] = v
	//	}
	//}

	return common.Marshal(baseMap)
}

func GetJSONFieldNames(t reflect.Type) map[string]struct{} {
	fields := make(map[string]struct{})
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 跳过匿名字段（例如 ExtraFields）
		if field.Anonymous {
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "-" || tag == "" {
			continue
		}

		// 取逗号前字段名（排除 omitempty 等）
		name := tag
		if commaIdx := indexComma(tag); commaIdx != -1 {
			name = tag[:commaIdx]
		}
		fields[name] = struct{}{}
	}
	return fields
}

func indexComma(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			return i
		}
	}
	return -1
}

func (i *ImageRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var sizeRatio = 1.0
	var qualityRatio = 1.0

	if strings.HasPrefix(i.Model, "dall-e") {
		// Size
		if i.Size == "256x256" {
			sizeRatio = 0.4
		} else if i.Size == "512x512" {
			sizeRatio = 0.45
		} else if i.Size == "1024x1024" {
			sizeRatio = 1
		} else if i.Size == "1024x1792" || i.Size == "1792x1024" {
			sizeRatio = 2
		}

		if i.Model == "dall-e-3" && i.Quality == "hd" {
			qualityRatio = 2.0
			if i.Size == "1024x1792" || i.Size == "1792x1024" {
				qualityRatio = 1.5
			}
		}
	}

	// n is NOT included here; it is handled via OtherRatio("n") in
	// image_handler.go (default) or channel adaptors (actual count).
	// Including n here caused double-counting for channels that also
	// set OtherRatio("n") (e.g. Ali/Bailian).
	return &types.TokenCountMeta{
		CombineText:     i.Prompt,
		MaxTokens:       1584,
		ImagePriceRatio: sizeRatio * qualityRatio,
	}
}

func (i *ImageRequest) IsStream(c *gin.Context) bool {
	return false
}

func (i *ImageRequest) SetModelName(modelName string) {
	if modelName != "" {
		i.Model = modelName
	}
}

type ImageResponse struct {
	Data     []ImageData     `json:"data"`
	Created  int64           `json:"created"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}
type ImageData struct {
	Url           string `json:"url"`
	B64Json       string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt"`
}
