package ratio_setting

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

const (
	ModelPriceVariantResolution = "resolution"
	ModelPriceVariantQuality    = "quality"
)

// ModelPriceVariantRule 使用请求规格匹配一个最终固定单价。
// Price 的单位与 ModelPriceUnit 一致：request 表示 $/次，second 表示 $/秒。
type ModelPriceVariantRule struct {
	Resolution string   `json:"resolution,omitempty"`
	Quality    string   `json:"quality,omitempty"`
	Price      *float64 `json:"price"`
}

// ModelPriceVariantConfig 声明固定价格是否随请求规格变化。
// 即使两个开关都关闭，配置本身仍有意义：它会明确关闭适配器内置的隐藏规格倍率。
type ModelPriceVariantConfig struct {
	ResolutionEnabled bool                    `json:"resolution_enabled"`
	QualityEnabled    bool                    `json:"quality_enabled"`
	Rules             []ModelPriceVariantRule `json:"rules,omitempty"`
	Inherited         bool                    `json:"inherited,omitempty"`
}

type ModelPriceVariantMatch struct {
	Configured bool
	Matched    bool
	Price      float64
	Resolution string
	Quality    string
}

type defaultVariantRatios struct {
	Resolution map[string]float64
}

// 内置配置只保存相对关系，实际展示价格根据管理员当前 ModelPrice 动态生成。
// 这样升级后仍保持既有自定义基础价及其分辨率倍率，不会突然回到官方绝对价格。
var defaultModelPriceVariantRatios = map[string]defaultVariantRatios{
	"grok-imagine-video": {
		Resolution: map[string]float64{
			"480p": 1,
			"720p": 0.07 / 0.05,
		},
	},
	"grok-imagine-video-1.5": {
		Resolution: map[string]float64{
			"480p":  1,
			"720p":  0.14 / 0.08,
			"1080p": 0.25 / 0.08,
		},
	},
}

// 这里只保存管理员显式覆盖项；内置配置由 getter 动态合并。
var modelPriceVariantOverrideMap = types.NewRWMap[string, ModelPriceVariantConfig]()

func normalizeVariantResolution(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "480", "480p", "sd":
		return "480p"
	case "720", "720p", "hd":
		return "720p"
	case "1080", "1080p", "fhd", "full-hd", "full_hd":
		return "1080p"
	case "2k":
		return "2k"
	case "4k":
		return "4k"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeVariantQuality(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeModelPriceVariantConfig(modelName string, config ModelPriceVariantConfig) (ModelPriceVariantConfig, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return ModelPriceVariantConfig{}, fmt.Errorf("规格差异计费的模型名称不能为空")
	}
	if !config.ResolutionEnabled && !config.QualityEnabled {
		config.Rules = nil
		return config, nil
	}
	if len(config.Rules) == 0 {
		return ModelPriceVariantConfig{}, fmt.Errorf("模型 %s 已开启规格差异计费，但没有配置任何档位价格", modelName)
	}

	normalizedRules := make([]ModelPriceVariantRule, 0, len(config.Rules))
	seen := make(map[string]struct{}, len(config.Rules))
	for index, rule := range config.Rules {
		if rule.Price == nil || math.IsNaN(*rule.Price) || math.IsInf(*rule.Price, 0) || *rule.Price < 0 {
			return ModelPriceVariantConfig{}, fmt.Errorf("模型 %s 第 %d 个规格价格无效", modelName, index+1)
		}
		if config.ResolutionEnabled {
			rule.Resolution = normalizeVariantResolution(rule.Resolution)
			if rule.Resolution == "" {
				return ModelPriceVariantConfig{}, fmt.Errorf("模型 %s 第 %d 个规格缺少分辨率", modelName, index+1)
			}
		} else {
			rule.Resolution = ""
		}
		if config.QualityEnabled {
			rule.Quality = normalizeVariantQuality(rule.Quality)
			if rule.Quality == "" {
				return ModelPriceVariantConfig{}, fmt.Errorf("模型 %s 第 %d 个规格缺少质量档位", modelName, index+1)
			}
		} else {
			rule.Quality = ""
		}

		key := rule.Resolution + "\x00" + rule.Quality
		if _, exists := seen[key]; exists {
			return ModelPriceVariantConfig{}, fmt.Errorf("模型 %s 存在重复的规格价格: %s/%s", modelName, rule.Resolution, rule.Quality)
		}
		seen[key] = struct{}{}
		normalizedRules = append(normalizedRules, rule)
	}
	config.Rules = normalizedRules
	return config, nil
}

func parseModelPriceVariants(jsonStr string) (map[string]ModelPriceVariantConfig, error) {
	configs := make(map[string]ModelPriceVariantConfig)
	if err := common.UnmarshalJsonStr(jsonStr, &configs); err != nil {
		return nil, err
	}
	if configs == nil {
		return nil, fmt.Errorf("规格差异计费配置必须是 JSON 对象")
	}
	normalized := make(map[string]ModelPriceVariantConfig, len(configs))
	for modelName, config := range configs {
		trimmedName := strings.TrimSpace(modelName)
		// 管理端会展示内置有效配置；未修改的继承项随整表回传时不能固化为用户覆盖。
		if config.Inherited {
			continue
		}
		normalizedConfig, err := normalizeModelPriceVariantConfig(trimmedName, config)
		if err != nil {
			return nil, err
		}
		normalized[trimmedName] = normalizedConfig
	}
	return normalized, nil
}

func CheckModelPriceVariantsJSONString(jsonStr string) error {
	_, err := parseModelPriceVariants(jsonStr)
	return err
}

func UpdateModelPriceVariantsByJSONString(jsonStr string) error {
	configs, err := parseModelPriceVariants(jsonStr)
	if err != nil {
		return err
	}
	normalized, err := common.Marshal(configs)
	if err != nil {
		return err
	}
	return types.LoadFromJsonStringWithCallback(modelPriceVariantOverrideMap, string(normalized), InvalidateExposedDataCache)
}

func cloneModelPriceVariantConfig(config ModelPriceVariantConfig) ModelPriceVariantConfig {
	cloned := config
	cloned.Rules = make([]ModelPriceVariantRule, len(config.Rules))
	for index, rule := range config.Rules {
		cloned.Rules[index] = rule
		if rule.Price != nil {
			price := *rule.Price
			cloned.Rules[index].Price = &price
		}
	}
	return cloned
}

func buildDefaultModelPriceVariantConfig(modelName string) (ModelPriceVariantConfig, bool) {
	defaults, ok := defaultModelPriceVariantRatios[modelName]
	if !ok {
		return ModelPriceVariantConfig{}, false
	}
	basePrice, ok := GetModelPrice(modelName, false)
	if !ok || basePrice < 0 {
		basePrice = defaultModelPrice[modelName]
	}
	resolutions := make([]string, 0, len(defaults.Resolution))
	for resolution := range defaults.Resolution {
		resolutions = append(resolutions, resolution)
	}
	resolutionOrder := map[string]int{"480p": 1, "720p": 2, "1080p": 3, "2k": 4, "4k": 5}
	sort.Slice(resolutions, func(i, j int) bool {
		left, leftKnown := resolutionOrder[resolutions[i]]
		right, rightKnown := resolutionOrder[resolutions[j]]
		if leftKnown && rightKnown {
			return left < right
		}
		if leftKnown != rightKnown {
			return leftKnown
		}
		return resolutions[i] < resolutions[j]
	})
	rules := make([]ModelPriceVariantRule, 0, len(resolutions))
	for _, resolution := range resolutions {
		price := basePrice * defaults.Resolution[resolution]
		rules = append(rules, ModelPriceVariantRule{
			Resolution: resolution,
			Price:      &price,
		})
	}
	return ModelPriceVariantConfig{ResolutionEnabled: true, Rules: rules, Inherited: true}, true
}

func GetModelPriceVariantConfig(modelName string) (ModelPriceVariantConfig, bool) {
	modelName = FormatMatchingModelName(modelName)
	if config, ok := modelPriceVariantOverrideMap.Get(modelName); ok {
		return cloneModelPriceVariantConfig(config), true
	}
	config, ok := buildDefaultModelPriceVariantConfig(modelName)
	return cloneModelPriceVariantConfig(config), ok
}

func GetModelPriceVariantsCopy() map[string]ModelPriceVariantConfig {
	configs := make(map[string]ModelPriceVariantConfig)
	for modelName := range defaultModelPriceVariantRatios {
		if config, ok := buildDefaultModelPriceVariantConfig(modelName); ok {
			configs[modelName] = config
		}
	}
	for modelName, config := range modelPriceVariantOverrideMap.ReadAll() {
		configs[modelName] = cloneModelPriceVariantConfig(config)
	}
	return configs
}

func ModelPriceVariants2JSONString() string {
	data, err := common.Marshal(GetModelPriceVariantsCopy())
	if err != nil {
		return "{}"
	}
	return string(data)
}

func MatchModelPriceVariant(modelName string, dimensions map[string]string) ModelPriceVariantMatch {
	config, configured := GetModelPriceVariantConfig(modelName)
	result := ModelPriceVariantMatch{Configured: configured}
	if !configured {
		return result
	}

	result.Resolution = normalizeVariantResolution(dimensions[ModelPriceVariantResolution])
	result.Quality = normalizeVariantQuality(dimensions[ModelPriceVariantQuality])
	if !config.ResolutionEnabled && !config.QualityEnabled {
		return result
	}
	if config.ResolutionEnabled && result.Resolution == "" {
		return result
	}
	if config.QualityEnabled && result.Quality == "" {
		return result
	}
	for _, rule := range config.Rules {
		if config.ResolutionEnabled && rule.Resolution != result.Resolution {
			continue
		}
		if config.QualityEnabled && rule.Quality != result.Quality {
			continue
		}
		result.Matched = true
		result.Price = *rule.Price
		return result
	}
	return result
}
