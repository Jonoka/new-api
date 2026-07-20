package setting

import (
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

const DefaultAutoGroupDescription = "自动选择最佳可用分组，失败时按配置顺序切换"

// AutoGroupConfig 保存 auto 虚拟令牌分组的展示与可选状态。
// auto 本身不是实体分组，实际路由目标仍由 AutoGroups 决定。
type AutoGroupConfig struct {
	UserSelectable bool   `json:"user_selectable"`
	Description    string `json:"description"`
}

var autoGroups = []string{
	"default",
}

var DefaultUseAutoGroup = false

var autoGroupConfig = AutoGroupConfig{
	UserSelectable: false,
	Description:    DefaultAutoGroupDescription,
}
var autoGroupConfigMutex sync.RWMutex

func NormalizeAutoGroupConfig(config AutoGroupConfig) AutoGroupConfig {
	config.Description = strings.TrimSpace(config.Description)
	if config.Description == "" {
		config.Description = DefaultAutoGroupDescription
	}
	return config
}

func GetAutoGroupConfig() AutoGroupConfig {
	autoGroupConfigMutex.RLock()
	defer autoGroupConfigMutex.RUnlock()
	return autoGroupConfig
}

func UpdateAutoGroupConfigByJsonString(jsonString string) error {
	var config AutoGroupConfig
	if err := common.UnmarshalJsonStr(jsonString, &config); err != nil {
		return err
	}
	config = NormalizeAutoGroupConfig(config)
	autoGroupConfigMutex.Lock()
	autoGroupConfig = config
	autoGroupConfigMutex.Unlock()
	return nil
}

func AutoGroupConfig2JsonString() string {
	config := GetAutoGroupConfig()
	jsonBytes, err := common.Marshal(config)
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

func ContainsAutoGroup(group string) bool {
	for _, autoGroup := range autoGroups {
		if autoGroup == group {
			return true
		}
	}
	return false
}

func UpdateAutoGroupsByJsonString(jsonString string) error {
	autoGroups = make([]string, 0)
	return common.Unmarshal([]byte(jsonString), &autoGroups)
}

func AutoGroups2JsonString() string {
	jsonBytes, err := common.Marshal(autoGroups)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func GetAutoGroups() []string {
	return autoGroups
}
