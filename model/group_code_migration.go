package model

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GroupCodeMigrationItem 描述一个旧 code 到稳定数字 code 的迁移项。
type GroupCodeMigrationItem struct {
	GroupID    int    `json:"group_id"`
	Name       string `json:"name"`
	OldCode    string `json:"old_code"`
	TargetCode string `json:"target_code"`
}

// GroupCodeMigrationSummary 同时用于预检和执行结果。
type GroupCodeMigrationSummary struct {
	CanExecute                bool                     `json:"can_execute"`
	Executed                  bool                     `json:"executed"`
	Groups                    []GroupCodeMigrationItem `json:"groups"`
	Blockers                  []string                 `json:"blockers,omitempty"`
	AffectedChannels          int64                    `json:"affected_channels"`
	AffectedTokens            int64                    `json:"affected_tokens"`
	AffectedUsers             int64                    `json:"affected_users"`
	AffectedAbilities         int64                    `json:"affected_abilities"`
	AffectedSubscriptionPlans int64                    `json:"affected_subscription_plans"`
	AffectedSubscriptions     int64                    `json:"affected_subscriptions"`
	AffectedOptions           int                      `json:"affected_options"`
	CacheInvalidated          int                      `json:"cache_invalidated"`
	CacheInvalidationFailed   int                      `json:"cache_invalidation_failed"`
	Warning                   string                   `json:"warning,omitempty"`
}

type groupCodeMigrationPlan struct {
	summary      *GroupCodeMigrationSummary
	replacements map[string]string
	groupIDs     []int
}

func uniqueSortedGroupCodeMigrationStrings(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func groupCodeMigrationCandidates(tx *gorm.DB, lock bool) ([]Group, error) {
	query := tx.Model(&Group{}).Order("id ASC")
	if lock {
		query = lockForUpdate(query)
	}
	var groups []Group
	if err := query.Find(&groups).Error; err != nil {
		return nil, err
	}
	candidates := make([]Group, 0)
	for _, group := range groups {
		if group.Code == "default" || isVirtualAutoCode(group.Code) || group.Code == strconv.Itoa(group.Id) {
			continue
		}
		candidates = append(candidates, group)
	}
	return candidates, nil
}

func buildGroupCodeMigrationPlan(tx *gorm.DB, lock bool) (*groupCodeMigrationPlan, error) {
	candidates, err := groupCodeMigrationCandidates(tx, lock)
	if err != nil {
		return nil, err
	}
	plan := &groupCodeMigrationPlan{
		summary:      &GroupCodeMigrationSummary{Groups: make([]GroupCodeMigrationItem, 0, len(candidates))},
		replacements: make(map[string]string, len(candidates)),
		groupIDs:     make([]int, 0, len(candidates)),
	}
	if len(candidates) == 0 {
		plan.summary.CanExecute = true
		return plan, nil
	}
	plan.summary.Warning = "执行迁移时只能保留当前一个新版本实例，其他实例及相关配置写入必须停止；完成后再统一启动全部新版本实例"

	var allGroups []Group
	if err := tx.Model(&Group{}).Order("id ASC").Find(&allGroups).Error; err != nil {
		return nil, err
	}
	var aliases []GroupAlias
	aliasQuery := tx.Model(&GroupAlias{}).Order("id ASC")
	if lock {
		aliasQuery = lockForUpdate(aliasQuery)
	}
	if err := aliasQuery.Find(&aliases).Error; err != nil {
		return nil, err
	}
	codeOwner := make(map[string]int, len(allGroups))
	aliasOwner := make(map[string]int, len(aliases))
	for _, group := range allGroups {
		codeOwner[group.Code] = group.Id
	}
	for _, alias := range aliases {
		aliasOwner[alias.Alias] = alias.GroupId
	}
	candidateByID := make(map[int]Group, len(candidates))
	for _, group := range candidates {
		candidateByID[group.Id] = group
	}

	for _, group := range candidates {
		target := strconv.Itoa(group.Id)
		plan.summary.Groups = append(plan.summary.Groups, GroupCodeMigrationItem{
			GroupID: group.Id, Name: group.Name, OldCode: group.Code, TargetCode: target,
		})
		plan.replacements[group.Code] = target
		plan.groupIDs = append(plan.groupIDs, group.Id)
		if owner, exists := codeOwner[target]; exists && owner != group.Id {
			plan.summary.Blockers = append(plan.summary.Blockers,
				fmt.Sprintf("分组 %s（ID %d）的目标标识 %s 已被分组 ID %d 使用", group.Name, group.Id, target, owner))
		}
		if owner, exists := aliasOwner[target]; exists && owner != group.Id {
			plan.summary.Blockers = append(plan.summary.Blockers,
				fmt.Sprintf("分组 %s（ID %d）的目标标识 %s 已是分组 ID %d 的历史别名", group.Name, group.Id, target, owner))
		}
		if owner, exists := aliasOwner[group.Code]; exists && owner != group.Id {
			plan.summary.Blockers = append(plan.summary.Blockers,
				fmt.Sprintf("旧标识 %s 已归属于分组 ID %d 的历史别名", group.Code, owner))
		}
		for otherID := range candidateByID {
			if otherID != group.Id && group.Code == strconv.Itoa(otherID) {
				plan.summary.Blockers = append(plan.summary.Blockers,
					fmt.Sprintf("旧标识 %s 与分组 ID %d 的迁移目标冲突，无法保留其历史别名", group.Code, otherID))
			}
		}
	}

	if err := populateGroupCodeMigrationImpact(tx, plan); err != nil {
		return nil, err
	}
	if err := validateGroupCodeMigrationAbilities(tx, plan); err != nil {
		plan.summary.Blockers = append(plan.summary.Blockers, err.Error())
	}
	if _, err := rewriteGroupCodeMigrationOptions(tx, plan.replacements); err != nil {
		plan.summary.Blockers = append(plan.summary.Blockers, err.Error())
	}
	plan.summary.Blockers = uniqueSortedGroupCodeMigrationStrings(plan.summary.Blockers)
	plan.summary.CanExecute = len(plan.summary.Blockers) == 0
	return plan, nil
}

func countRowsByGroupIDs(tx *gorm.DB, entity interface{}, ids []int) (int64, error) {
	if len(ids) == 0 || !hasModelColumns(tx, entity, "GroupId") {
		return 0, nil
	}
	var count int64
	if err := tx.Unscoped().Model(entity).Where("group_id IN ?", ids).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func countDistinctBindingOwners(tx *gorm.DB, entity interface{}, ownerColumn string, ids []int) (int64, error) {
	if len(ids) == 0 || !hasModelColumns(tx, entity, "GroupId") {
		return 0, nil
	}
	var owners []int
	if err := tx.Model(entity).Where("group_id IN ?", ids).Distinct(ownerColumn).Pluck(ownerColumn, &owners).Error; err != nil {
		return 0, err
	}
	return int64(len(owners)), nil
}

func countRowsByCodes(tx *gorm.DB, entity interface{}, field, column string, codes []string) (int64, error) {
	if len(codes) == 0 || !hasModelColumns(tx, entity, field) {
		return 0, nil
	}
	var count int64
	if err := tx.Unscoped().Model(entity).Where(column+" IN ?", codes).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func populateGroupCodeMigrationImpact(tx *gorm.DB, plan *groupCodeMigrationPlan) error {
	if len(plan.groupIDs) == 0 {
		return nil
	}
	oldCodes := make([]string, 0, len(plan.replacements))
	for oldCode := range plan.replacements {
		oldCodes = append(oldCodes, oldCode)
	}
	var err error
	if plan.summary.AffectedChannels, err = countDistinctBindingOwners(tx, &ChannelGroupBinding{}, "channel_id", plan.groupIDs); err != nil {
		return err
	}
	if plan.summary.AffectedTokens, err = countDistinctBindingOwners(tx, &TokenGroupBinding{}, "token_id", plan.groupIDs); err != nil {
		return err
	}
	if plan.summary.AffectedUsers, err = countRowsByGroupIDs(tx, &User{}, plan.groupIDs); err != nil {
		return err
	}
	if plan.summary.AffectedAbilities, err = countRowsByGroupIDs(tx, &Ability{}, plan.groupIDs); err != nil {
		return err
	}
	if plan.summary.AffectedSubscriptionPlans, err = countRowsByCodes(tx, &SubscriptionPlan{}, "UpgradeGroup", "upgrade_group", oldCodes); err != nil {
		return err
	}
	if hasModelColumns(tx, &UserSubscription{}, "UpgradeGroup", "PrevUserGroup") {
		var subscriptionIDs []int
		if err := tx.Unscoped().Model(&UserSubscription{}).
			Where("upgrade_group IN ? OR prev_user_group IN ?", oldCodes, oldCodes).
			Distinct("id").
			Pluck("id", &subscriptionIDs).Error; err != nil {
			return err
		}
		plan.summary.AffectedSubscriptions = int64(len(subscriptionIDs))
	}
	updates, err := rewriteGroupCodeMigrationOptions(tx, plan.replacements)
	if err != nil {
		return err
	}
	plan.summary.AffectedOptions = len(updates) + len(groupProjectionOptionKeys)
	return nil
}

func validateGroupCodeMigrationAbilities(tx *gorm.DB, plan *groupCodeMigrationPlan) error {
	if len(plan.groupIDs) == 0 || !hasModelColumns(tx, &Ability{}, "GroupId", "Group", "Model", "ChannelId") {
		return nil
	}
	var abilities []Ability
	if err := tx.Where("group_id IN ?", plan.groupIDs).Find(&abilities).Error; err != nil {
		return err
	}
	seen := make(map[string]string, len(abilities))
	targetCodes := make([]string, 0, len(plan.summary.Groups))
	for _, item := range plan.summary.Groups {
		targetCodes = append(targetCodes, item.TargetCode)
	}
	for _, ability := range abilities {
		key := fmt.Sprintf("%d\x00%s\x00%s", ability.ChannelId, ability.Model, strconv.Itoa(ability.GroupId))
		if previous, exists := seen[key]; exists && previous != ability.Group {
			return fmt.Errorf("能力表存在同一分组、渠道和模型的重复旧标识 %q 与 %q，请先清理冲突", previous, ability.Group)
		}
		seen[key] = ability.Group
	}

	var targetHolders []Ability
	if err := tx.Where(commonGroupCol+" IN ?", targetCodes).Find(&targetHolders).Error; err != nil {
		return err
	}
	for _, holder := range targetHolders {
		key := fmt.Sprintf("%d\x00%s\x00%s", holder.ChannelId, holder.Model, holder.Group)
		if sourceGroup, migrating := seen[key]; migrating {
			expectedGroupID, parseErr := strconv.Atoi(holder.Group)
			if parseErr == nil && holder.GroupId == expectedGroupID && sourceGroup == holder.Group {
				continue
			}
			return fmt.Errorf(
				"能力表目标标识 %q 在渠道 %d、模型 %s 上已被分组 ID %d 使用",
				holder.Group,
				holder.ChannelId,
				holder.Model,
				holder.GroupId,
			)
		}
	}
	return nil
}

func rewriteSimpleMapKeys[T any](values map[string]T, replacements map[string]string, key string) (map[string]T, error) {
	result := make(map[string]T, len(values))
	sources := make(map[string]string, len(values))
	for source, value := range values {
		target := rewrittenTemporaryGroupCode(source, replacements)
		if previous, exists := sources[target]; exists && previous != source {
			return nil, fmt.Errorf("分组选项 %s 的 %q 与 %q 迁移后发生冲突", key, previous, source)
		}
		sources[target] = source
		result[target] = value
	}
	return result, nil
}

func rewriteRateLimitUserGroups(value string, replacements map[string]string) (string, error) {
	values := make(map[string]setting.UserGroupRateLimit)
	if err := common.UnmarshalJsonStr(value, &values); err != nil {
		return "", err
	}
	rewritten := make(map[string]setting.UserGroupRateLimit, len(values))
	ownerSources := make(map[string]string, len(values))
	for owner, config := range values {
		newOwner := rewrittenTemporaryGroupCode(owner, replacements)
		if previous, exists := ownerSources[newOwner]; exists && previous != owner {
			return "", fmt.Errorf("分组选项 ModelRequestRateLimitUserGroup 的 %q 与 %q 迁移后发生冲突", previous, owner)
		}
		ownerSources[newOwner] = owner
		groups, err := rewriteSimpleMapKeys(config.Groups, replacements, "ModelRequestRateLimitUserGroup.groups")
		if err != nil {
			return "", err
		}
		config.Groups = groups
		rewritten[newOwner] = config
	}
	raw, err := common.Marshal(rewritten)
	return string(raw), err
}

func rewriteGroupCodeMigrationOptions(tx *gorm.DB, replacements map[string]string) (map[string]string, error) {
	if len(replacements) == 0 {
		return map[string]string{}, nil
	}
	keys := []string{groupGroupRatioOptionKey, layeredGroupGroupRatioOptionKey, "TopupGroupRatio", "group_ratio_setting.group_special_usable_group", "ModelRequestRateLimitGroup", "ModelRequestRateLimitUserGroup"}
	var options []Option
	if err := tx.Where(commonKeyCol+" IN ?", keys).Find(&options).Error; err != nil {
		return nil, err
	}
	updates := make(map[string]string)
	for _, option := range options {
		if strings.TrimSpace(option.Value) == "" {
			continue
		}
		var next string
		var err error
		switch option.Key {
		case groupGroupRatioOptionKey, layeredGroupGroupRatioOptionKey:
			next, err = rewriteTemporaryGroupRatioReferences(option.Key, option.Value, replacements)
		case "TopupGroupRatio":
			next, err = rewriteTemporaryTopupGroupRatioReferences(option.Key, option.Value, replacements)
		case "group_ratio_setting.group_special_usable_group":
			next, err = rewriteTemporarySpecialUsableGroupReferences(option.Key, option.Value, replacements)
		case "ModelRequestRateLimitGroup":
			var values map[string]setting.RateLimitCounts
			if err = common.UnmarshalJsonStr(option.Value, &values); err == nil {
				var rewritten map[string]setting.RateLimitCounts
				rewritten, err = rewriteSimpleMapKeys(values, replacements, option.Key)
				if err == nil {
					var raw []byte
					raw, err = common.Marshal(rewritten)
					next = string(raw)
				}
			}
		case "ModelRequestRateLimitUserGroup":
			next, err = rewriteRateLimitUserGroups(option.Value, replacements)
		}
		if err != nil {
			return nil, fmt.Errorf("迁移分组选项 %s 失败: %w", option.Key, err)
		}
		if next != option.Value {
			updates[option.Key] = next
		}
	}
	return updates, nil
}

// PreviewGroupCodeMigration 预检所有非 default 旧 code。
func PreviewGroupCodeMigration() (*GroupCodeMigrationSummary, error) {
	if DB == nil {
		return nil, errors.New("database is nil")
	}
	plan, err := buildGroupCodeMigrationPlan(DB, false)
	if err != nil {
		return nil, err
	}
	return plan.summary, nil
}

func applyGroupCodeMigration(tx *gorm.DB, plan *groupCodeMigrationPlan) (map[string]string, []Token, []int, error) {
	if len(plan.groupIDs) == 0 {
		return map[string]string{}, nil, nil, nil
	}
	now := time.Now().Unix()
	for _, item := range plan.summary.Groups {
		placeholder := fmt.Sprintf("__gcm_%d_%d", item.GroupID, time.Now().UnixNano())
		if err := tx.Model(&Group{}).Where("id = ?", item.GroupID).Updates(map[string]interface{}{"code": placeholder, "updated_time": now}).Error; err != nil {
			return nil, nil, nil, err
		}
	}
	for _, item := range plan.summary.Groups {
		if err := tx.Model(&Group{}).Where("id = ?", item.GroupID).Updates(map[string]interface{}{"code": item.TargetCode, "updated_time": now}).Error; err != nil {
			return nil, nil, nil, err
		}
		if err := tx.Where("alias = ? AND group_id = ?", item.TargetCode, item.GroupID).Delete(&GroupAlias{}).Error; err != nil {
			return nil, nil, nil, err
		}
		alias := GroupAlias{Alias: item.OldCode, GroupId: item.GroupID, CreatedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&alias).Error; err != nil {
			return nil, nil, nil, err
		}
	}

	var abilities []Ability
	if err := tx.Where("group_id IN ?", plan.groupIDs).Find(&abilities).Error; err != nil {
		return nil, nil, nil, err
	}
	abilityNonce := time.Now().UnixNano()
	for index, ability := range abilities {
		stage := ""
		for attempt := 0; attempt < 100; attempt++ {
			candidate := fmt.Sprintf("__gca_%d_%d_%d", abilityNonce, index, attempt)
			var count int64
			if err := tx.Model(&Ability{}).
				Where("channel_id = ? AND model = ? AND "+commonGroupCol+" = ?", ability.ChannelId, ability.Model, candidate).
				Count(&count).Error; err != nil {
				return nil, nil, nil, err
			}
			if count == 0 {
				stage = candidate
				break
			}
		}
		if stage == "" {
			return nil, nil, nil, fmt.Errorf("无法为渠道 %d、模型 %s 的能力记录分配事务占位标识", ability.ChannelId, ability.Model)
		}
		result := tx.Model(&Ability{}).
			Where("channel_id = ? AND model = ? AND "+commonGroupCol+" = ?", ability.ChannelId, ability.Model, ability.Group).
			Update("group", stage)
		if result.Error != nil {
			return nil, nil, nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, nil, nil, fmt.Errorf("能力记录在迁移期间发生变化，请重试")
		}
		abilities[index].Group = stage
	}
	for _, ability := range abilities {
		finalCode := strconv.Itoa(ability.GroupId)
		if err := tx.Model(&Ability{}).Where("channel_id = ? AND model = ? AND "+commonGroupCol+" = ?", ability.ChannelId, ability.Model, ability.Group).Update("group", finalCode).Error; err != nil {
			return nil, nil, nil, err
		}
	}

	channelIDs := make([]int, 0)
	if err := tx.Model(&ChannelGroupBinding{}).Where("group_id IN ?", plan.groupIDs).Distinct("channel_id").Pluck("channel_id", &channelIDs).Error; err != nil {
		return nil, nil, nil, err
	}
	if len(channelIDs) > 0 {
		var channels []*Channel
		if err := lockForUpdate(tx.Model(&Channel{})).Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
			return nil, nil, nil, err
		}
		if err := HydrateChannelGroupBindings(tx, channels); err != nil {
			return nil, nil, nil, err
		}
		for _, channel := range channels {
			codes := make([]string, 0, len(channel.GroupDetails))
			for _, detail := range channel.GroupDetails {
				codes = append(codes, detail.Code)
			}
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Update("group", strings.Join(codes, ",")).Error; err != nil {
				return nil, nil, nil, err
			}
		}
	}

	tokenIDs := make([]int, 0)
	if err := tx.Model(&TokenGroupBinding{}).Where("group_id IN ?", plan.groupIDs).Distinct("token_id").Pluck("token_id", &tokenIDs).Error; err != nil {
		return nil, nil, nil, err
	}
	var affectedTokens []Token
	if len(tokenIDs) > 0 {
		if err := lockForUpdate(tx.Unscoped().Model(&Token{})).Where("id IN ?", tokenIDs).Find(&affectedTokens).Error; err != nil {
			return nil, nil, nil, err
		}
		pointers := make([]*Token, 0, len(affectedTokens))
		for index := range affectedTokens {
			pointers = append(pointers, &affectedTokens[index])
		}
		if err := HydrateTokenGroupBindings(tx, pointers); err != nil {
			return nil, nil, nil, err
		}
		var bindings []TokenGroupBinding
		if err := tx.Where("token_id IN ?", tokenIDs).Order("token_id ASC, position ASC").Find(&bindings).Error; err != nil {
			return nil, nil, nil, err
		}
		limitsByToken := make(map[int]map[string]float64)
		for _, binding := range bindings {
			if binding.RatioLimit == nil {
				continue
			}
			groupCode := strconv.Itoa(binding.GroupId)
			var group Group
			if err := tx.Select("code").First(&group, "id = ?", binding.GroupId).Error; err != nil {
				return nil, nil, nil, err
			}
			groupCode = group.Code
			if limitsByToken[binding.TokenId] == nil {
				limitsByToken[binding.TokenId] = make(map[string]float64)
			}
			limitsByToken[binding.TokenId][groupCode] = *binding.RatioLimit
		}
		for index := range affectedTokens {
			token := &affectedTokens[index]
			codes := make([]string, 0, len(token.GroupDetails))
			for _, detail := range token.GroupDetails {
				codes = append(codes, detail.Code)
			}
			limits, err := marshalTokenGroupRatioLimitsForMigration(limitsByToken[token.Id])
			if err != nil {
				return nil, nil, nil, err
			}
			if err := tx.Unscoped().Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{"group": strings.Join(codes, ","), "group_mode": TokenGroupModeExplicit, "group_ratio_limits": limits}).Error; err != nil {
				return nil, nil, nil, err
			}
		}
	}

	userIDs := make([]int, 0)
	if err := tx.Unscoped().Model(&User{}).Where("group_id IN ?", plan.groupIDs).Pluck("id", &userIDs).Error; err != nil {
		return nil, nil, nil, err
	}
	for _, item := range plan.summary.Groups {
		if err := tx.Unscoped().Model(&User{}).Where("group_id = ?", item.GroupID).Update("group", item.TargetCode).Error; err != nil {
			return nil, nil, nil, err
		}
		if hasModelColumns(tx, &SubscriptionPlan{}, "UpgradeGroup") {
			if err := tx.Model(&SubscriptionPlan{}).Where("upgrade_group = ?", item.OldCode).Update("upgrade_group", item.TargetCode).Error; err != nil {
				return nil, nil, nil, err
			}
		}
		if hasModelColumns(tx, &UserSubscription{}, "UpgradeGroup", "PrevUserGroup") {
			if err := tx.Model(&UserSubscription{}).Where("upgrade_group = ?", item.OldCode).Update("upgrade_group", item.TargetCode).Error; err != nil {
				return nil, nil, nil, err
			}
			if err := tx.Model(&UserSubscription{}).Where("prev_user_group = ?", item.OldCode).Update("prev_user_group", item.TargetCode).Error; err != nil {
				return nil, nil, nil, err
			}
		}
	}

	optionUpdates, err := rewriteGroupCodeMigrationOptions(tx, plan.replacements)
	if err != nil {
		return nil, nil, nil, err
	}
	projection, err := buildGroupOptionProjection(tx)
	if err != nil {
		return nil, nil, nil, err
	}
	for key, value := range projection {
		optionUpdates[key] = value
	}
	keys := make([]string, 0, len(optionUpdates))
	for key := range optionUpdates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		option := Option{Key: key}
		if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
			return nil, nil, nil, err
		}
		if err := tx.Model(&Option{}).Where(commonKeyCol+" = ?", key).Update("value", optionUpdates[key]).Error; err != nil {
			return nil, nil, nil, err
		}
	}
	return optionUpdates, affectedTokens, userIDs, nil
}

// MigrateLegacyGroupCodesToIDs 原子迁移所有旧 code；调用方必须先展示预检结果。
func MigrateLegacyGroupCodesToIDs() (*GroupCodeMigrationSummary, error) {
	if DB == nil {
		return nil, errors.New("database is nil")
	}
	optionWriteMutex.Lock()
	defer optionWriteMutex.Unlock()
	var plan *groupCodeMigrationPlan
	var optionUpdates map[string]string
	var affectedTokens []Token
	var userIDs []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		plan, err = buildGroupCodeMigrationPlan(tx, true)
		if err != nil {
			return err
		}
		if !plan.summary.CanExecute {
			return fmt.Errorf("分组标识迁移预检失败: %s", strings.Join(plan.summary.Blockers, "；"))
		}
		optionUpdates, affectedTokens, userIDs, err = applyGroupCodeMigration(tx, plan)
		return err
	})
	if err != nil {
		return nil, err
	}
	plan.summary.Executed = true
	plan.summary.Warning = ""
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
	appendWarning := func(message string) {
		if strings.TrimSpace(message) == "" {
			return
		}
		if plan.summary.Warning == "" {
			plan.summary.Warning = message
			return
		}
		plan.summary.Warning += "；" + message
	}
	for key, value := range optionUpdates {
		if err := updateOptionMap(key, value); err != nil {
			appendWarning("数据库迁移已完成，但运行时选项刷新失败，请重启全部实例")
			common.SysLog(fmt.Sprintf("failed to refresh migrated group option %s: %v", key, err))
		}
	}
	InitChannelCache()
	InvalidateExclusiveGroupSnapshot()
	planIDs := make([]int, 0)
	targetCodes := make([]string, 0, len(plan.summary.Groups))
	for _, item := range plan.summary.Groups {
		targetCodes = append(targetCodes, item.TargetCode)
	}
	if len(targetCodes) > 0 && DB.Migrator().HasTable(&SubscriptionPlan{}) {
		if err := DB.Model(&SubscriptionPlan{}).Where("upgrade_group IN ?", targetCodes).Pluck("id", &planIDs).Error; err != nil {
			appendWarning("数据库迁移已完成，但套餐缓存刷新失败，请重启全部实例")
			common.SysLog(fmt.Sprintf("failed to load migrated subscription plans: %v", err))
		} else {
			for _, planID := range planIDs {
				InvalidateSubscriptionPlanCache(planID)
			}
		}
	}
	if common.RedisEnabled {
		keys := make([]string, 0, len(affectedTokens))
		for _, token := range affectedTokens {
			if !token.DeletedAt.Valid && token.Key != "" {
				keys = append(keys, token.Key)
			}
		}
		if err := cacheDeleteTokens(keys); err != nil {
			plan.summary.CacheInvalidationFailed += len(keys)
		} else {
			plan.summary.CacheInvalidated += len(keys)
		}
		for _, userID := range userIDs {
			if err := invalidateUserCache(userID); err != nil {
				plan.summary.CacheInvalidationFailed++
			} else {
				plan.summary.CacheInvalidated++
			}
		}
	}
	if plan.summary.CacheInvalidationFailed > 0 {
		appendWarning(fmt.Sprintf("数据库迁移已完成，但 %d 项缓存清理失败，请重启全部实例或清理 Redis 缓存", plan.summary.CacheInvalidationFailed))
	}
	return plan.summary, nil
}
