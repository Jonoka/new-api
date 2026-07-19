package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// TokenGroupMigrationSummary 是令牌分组迁移的预览和执行结果。
type TokenGroupMigrationSummary struct {
	SourceGroup             GroupReference `json:"source_group"`
	TargetGroup             GroupReference `json:"target_group"`
	MigratedTokens          int            `json:"migrated_tokens"`
	DeduplicatedTokens      int            `json:"deduplicated_tokens"`
	SingleGroupTokens       int            `json:"single_group_tokens"`
	MultiGroupTokens        int            `json:"multi_group_tokens"`
	AffectedUsers           int            `json:"affected_users"`
	CleanedDeletedTokens    int            `json:"cleaned_deleted_tokens"`
	CacheInvalidated        int            `json:"cache_invalidated"`
	CacheInvalidationFailed int            `json:"cache_invalidation_failed"`
	Warning                 string         `json:"warning,omitempty"`
}

type tokenGroupMigrationPlan struct {
	token Token
}

func validateTokenGroupMigration(tx *gorm.DB, sourceGroupID, targetGroupID int, lock bool) (*Group, *Group, error) {
	if sourceGroupID <= 0 || targetGroupID <= 0 {
		return nil, nil, errors.New("源分组和目标分组 ID 必须大于 0")
	}
	if sourceGroupID == targetGroupID {
		return nil, nil, errors.New("源分组和目标分组不能相同")
	}

	ids := []int{sourceGroupID, targetGroupID}
	sort.Ints(ids)
	query := tx.Model(&Group{}).Where("id IN ?", ids).Order("id ASC")
	if lock {
		query = lockForUpdate(query)
	}
	var groups []Group
	if err := query.Find(&groups).Error; err != nil {
		return nil, nil, err
	}
	byID := make(map[int]*Group, len(groups))
	for index := range groups {
		byID[groups[index].Id] = &groups[index]
	}
	sourceGroup := byID[sourceGroupID]
	if sourceGroup == nil {
		return nil, nil, fmt.Errorf("源分组 ID %d 不存在", sourceGroupID)
	}
	targetGroup := byID[targetGroupID]
	if targetGroup == nil {
		return nil, nil, fmt.Errorf("目标分组 ID %d 不存在", targetGroupID)
	}
	if targetGroup.Status != GroupStatusActive {
		return nil, nil, fmt.Errorf("目标分组 %s 已禁用，不能接收令牌", targetGroup.Name)
	}
	return sourceGroup, targetGroup, nil
}

func affectedTokensForGroupQuery(tx *gorm.DB, sourceGroupID int, legacyIdentifiers []string) *gorm.DB {
	boundTokenIDs := tx.Model(&TokenGroupBinding{}).
		Select("token_id").
		Where("group_id = ?", sourceGroupID)
	query := tx.Unscoped().Model(&Token{}).Where("id IN (?)", boundTokenIDs)
	for _, identifier := range legacyIdentifiers {
		query = query.Or(
			commonGroupCol+" LIKE ? ESCAPE '!'",
			legacyGroupSubstringPattern(identifier),
		)
	}
	return query
}

func parseTokenGroupRatioLimitsForMigration(token *Token) (map[string]float64, error) {
	limits := make(map[string]float64)
	if token == nil || strings.TrimSpace(token.GroupRatioLimits) == "" {
		return limits, nil
	}
	if err := common.UnmarshalJsonStr(token.GroupRatioLimits, &limits); err != nil {
		return nil, fmt.Errorf("令牌 %d 的倍率保护配置格式错误: %w", token.Id, err)
	}
	normalized := make(map[string]float64, len(limits))
	for code, ratio := range limits {
		code = strings.TrimSpace(code)
		if code == "" {
			return nil, fmt.Errorf("令牌 %d 的倍率保护分组标识不能为空", token.Id)
		}
		if ratio <= 0 {
			return nil, fmt.Errorf("令牌 %d 分组 %s 的倍率保护必须大于 0", token.Id, code)
		}
		if _, exists := normalized[code]; exists {
			return nil, fmt.Errorf("令牌 %d 的倍率保护分组 %s 重复", token.Id, code)
		}
		normalized[code] = ratio
	}
	return normalized, nil
}

func normalizeTokenGroupRatioLimitCodesForMigration(
	tx *gorm.DB,
	token *Token,
	limits map[string]float64,
) (map[string]float64, error) {
	normalized := make(map[string]float64, len(limits))
	for identifier, ratio := range limits {
		group, err := getGroupByCodeOrAliasStrict(tx, identifier)
		if err != nil {
			return nil, fmt.Errorf("令牌 %d 的倍率保护分组 %s 不存在: %w", token.Id, identifier, err)
		}
		if _, exists := normalized[group.Code]; exists {
			return nil, fmt.Errorf("令牌 %d 的倍率保护包含指向同一分组的重复标识", token.Id)
		}
		normalized[group.Code] = ratio
	}
	return normalized, nil
}

func marshalTokenGroupRatioLimitsForMigration(limits map[string]float64) (string, error) {
	if len(limits) == 0 {
		return "", nil
	}
	data, err := common.Marshal(limits)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func equalGroupIDSlices(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func loadTokenGroupMigrationTokensByIDs(tx *gorm.DB, tokenIDs []int, lock bool) ([]Token, error) {
	const batchSize = 500
	tokens := make([]Token, 0, len(tokenIDs))
	for start := 0; start < len(tokenIDs); start += batchSize {
		end := start + batchSize
		if end > len(tokenIDs) {
			end = len(tokenIDs)
		}
		query := tx.Unscoped().Model(&Token{}).
			Select("id", "user_id", commonKeyCol, groupBindingGroupColumn(), "group_mode", "group_ratio_limits", "deleted_at").
			Where("id IN ?", tokenIDs[start:end]).
			Order("id ASC")
		if lock {
			query = lockForUpdate(query)
		}
		var batch []Token
		if err := query.Find(&batch).Error; err != nil {
			return nil, err
		}
		tokens = append(tokens, batch...)
	}
	sort.Slice(tokens, func(i, j int) bool { return tokens[i].Id < tokens[j].Id })
	return tokens, nil
}

func loadTokenGroupMigrationBindingsByIDs(tx *gorm.DB, tokenIDs []int) ([]TokenGroupBinding, error) {
	const batchSize = 500
	bindings := make([]TokenGroupBinding, 0)
	for start := 0; start < len(tokenIDs); start += batchSize {
		end := start + batchSize
		if end > len(tokenIDs) {
			end = len(tokenIDs)
		}
		var batch []TokenGroupBinding
		if err := tx.Model(&TokenGroupBinding{}).
			Where("token_id IN ?", tokenIDs[start:end]).
			Find(&batch).Error; err != nil {
			return nil, err
		}
		bindings = append(bindings, batch...)
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].TokenId != bindings[j].TokenId {
			return bindings[i].TokenId < bindings[j].TokenId
		}
		return bindings[i].Position < bindings[j].Position
	})
	return bindings, nil
}

func buildTokenGroupMigrationPlans(
	tx *gorm.DB,
	sourceGroupID int,
	targetGroupID int,
	lock bool,
) ([]tokenGroupMigrationPlan, *TokenGroupMigrationSummary, error) {
	sourceGroup, targetGroup, err := validateTokenGroupMigration(tx, sourceGroupID, targetGroupID, lock)
	if err != nil {
		return nil, nil, err
	}

	legacyIdentifiers, legacyIdentifierSet, err := groupLegacyIdentifiers(tx, sourceGroup)
	if err != nil {
		return nil, nil, fmt.Errorf("加载源分组兼容标识失败: %w", err)
	}
	tokenQuery := affectedTokensForGroupQuery(tx, sourceGroupID, legacyIdentifiers).
		Select("id", "user_id", commonKeyCol, groupBindingGroupColumn(), "group_mode", "group_ratio_limits", "deleted_at").
		Order("id ASC")
	var tokens []Token
	if err := tokenQuery.Find(&tokens).Error; err != nil {
		return nil, nil, fmt.Errorf("加载待迁移令牌失败: %w", err)
	}

	summary := &TokenGroupMigrationSummary{
		SourceGroup: newGroupReference(sourceGroup),
		TargetGroup: newGroupReference(targetGroup),
	}
	if len(tokens) == 0 {
		return nil, summary, nil
	}

	affectedTokenIDs := affectedTokensForGroupQuery(tx, sourceGroupID, legacyIdentifiers).Select("id")
	var bindings []TokenGroupBinding
	if err := tx.Model(&TokenGroupBinding{}).
		Where("token_id IN (?)", affectedTokenIDs).
		Order("token_id ASC, position ASC").
		Find(&bindings).Error; err != nil {
		return nil, nil, fmt.Errorf("加载令牌分组绑定失败: %w", err)
	}
	bindingsByTokenID := make(map[int][]TokenGroupBinding)
	for _, binding := range bindings {
		bindingsByTokenID[binding.TokenId] = append(bindingsByTokenID[binding.TokenId], binding)
	}
	exactTokenIDs := make([]int, 0, len(tokens))
	exactTokens := make([]Token, 0, len(tokens))
	for _, token := range tokens {
		hasSourceBinding := false
		for _, binding := range bindingsByTokenID[token.Id] {
			if binding.GroupId == sourceGroupID {
				hasSourceBinding = true
				break
			}
		}
		if !hasSourceBinding && !containsLegacyGroupIdentifier(token.Group, legacyIdentifierSet) {
			continue
		}
		exactTokenIDs = append(exactTokenIDs, token.Id)
		exactTokens = append(exactTokens, token)
	}
	if len(exactTokenIDs) == 0 {
		return nil, summary, nil
	}
	if lock {
		tokens, err = loadTokenGroupMigrationTokensByIDs(tx, exactTokenIDs, true)
		if err != nil {
			return nil, nil, fmt.Errorf("锁定待迁移令牌失败: %w", err)
		}
		if len(tokens) != len(exactTokenIDs) {
			return nil, nil, errors.New("待迁移令牌在锁定期间发生变化，请重试")
		}
		bindings, err = loadTokenGroupMigrationBindingsByIDs(tx, exactTokenIDs)
		if err != nil {
			return nil, nil, fmt.Errorf("重新加载令牌分组绑定失败: %w", err)
		}
		bindingsByTokenID = make(map[int][]TokenGroupBinding, len(exactTokenIDs))
		for _, binding := range bindings {
			bindingsByTokenID[binding.TokenId] = append(bindingsByTokenID[binding.TokenId], binding)
		}
	} else {
		tokens = exactTokens
	}

	var allGroups []Group
	if err := tx.Model(&Group{}).Find(&allGroups).Error; err != nil {
		return nil, nil, fmt.Errorf("加载分组信息失败: %w", err)
	}
	groupsByID := make(map[int]*Group, len(allGroups))
	for index := range allGroups {
		groupsByID[allGroups[index].Id] = &allGroups[index]
	}

	plans := make([]tokenGroupMigrationPlan, 0, len(tokens))
	affectedUsers := make(map[int]struct{})
	for index := range tokens {
		token := &tokens[index]
		tokenBindings := bindingsByTokenID[token.Id]
		groupIDs := make([]int, 0, len(tokenBindings))
		bindingLimits := make(map[int]*float64, len(tokenBindings))
		containsSourceBinding := false
		for _, binding := range tokenBindings {
			groupIDs = append(groupIDs, binding.GroupId)
			bindingLimits[binding.GroupId] = binding.RatioLimit
			if binding.GroupId == sourceGroupID {
				containsSourceBinding = true
			}
		}
		hasLegacySource := containsLegacyGroupIdentifier(token.Group, legacyIdentifierSet)
		if !containsSourceBinding && !hasLegacySource {
			// SQL LIKE 仅用于跨数据库宽筛选；稳定 ID 和大小写敏感标识才是最终依据。
			continue
		}
		if token.DeletedAt.Valid {
			// 已删除令牌不会再参与鉴权。清空其历史分组镜像和残留稳定绑定，
			// 避免把源分组的删除阻塞转移到目标分组。
			token.Group = ""
			token.GroupMode = TokenGroupModeInherit
			token.GroupIds = nil
			token.GroupDetails = nil
			token.GroupRatioLimits = ""
			plans = append(plans, tokenGroupMigrationPlan{token: *token})
			summary.CleanedDeletedTokens++
			continue
		}

		limits, err := parseTokenGroupRatioLimitsForMigration(token)
		if err != nil {
			return nil, nil, err
		}
		limits, err = normalizeTokenGroupRatioLimitCodesForMigration(tx, token, limits)
		if err != nil {
			return nil, nil, err
		}
		for _, groupID := range groupIDs {
			if groupsByID[groupID] == nil {
				return nil, nil, fmt.Errorf("令牌 %d 绑定了不存在的分组 ID %d", token.Id, groupID)
			}
		}

		legacyIDs, _, _, resolveErr := resolveBindingGroupsWithPolicy(
			tx,
			nil,
			token.Group,
			groupBindingResolvePolicy{allowAllDisabled: true, tablesVerified: true},
		)
		if resolveErr != nil {
			return nil, nil, fmt.Errorf("解析令牌 %d 的历史分组失败: %w", token.Id, resolveErr)
		}
		if len(tokenBindings) == 0 {
			groupIDs = legacyIDs
			bindingLimits = make(map[int]*float64, len(legacyIDs))
		} else if !equalGroupIDSlices(groupIDs, legacyIDs) {
			return nil, nil, fmt.Errorf(
				"令牌 %d 的稳定分组绑定与旧分组镜像不一致，请先修复数据后再迁移",
				token.Id,
			)
		}
		if !containsSourceBinding && len(tokenBindings) > 0 {
			return nil, nil, fmt.Errorf("令牌 %d 的稳定绑定不包含源分组，请先修复数据后再迁移", token.Id)
		}

		if len(groupIDs) == 0 {
			return nil, nil, fmt.Errorf("令牌 %d 没有可迁移的显式分组", token.Id)
		}

		selectedCodes := make(map[string]struct{}, len(groupIDs))
		for _, groupID := range groupIDs {
			group := groupsByID[groupID]
			if group == nil {
				return nil, nil, fmt.Errorf("令牌 %d 绑定了不存在的分组 ID %d", token.Id, groupID)
			}
			selectedCodes[group.Code] = struct{}{}
			if len(tokenBindings) > 0 {
				bindingLimit := bindingLimits[groupID]
				jsonLimit, hasJSONLimit := limits[group.Code]
				if bindingLimit == nil && hasJSONLimit {
					return nil, nil, fmt.Errorf(
						"令牌 %d 分组 %s 的稳定倍率保护与旧镜像不一致，请先修复数据后再迁移",
						token.Id,
						group.Code,
					)
				}
				if bindingLimit != nil {
					if *bindingLimit <= 0 || !hasJSONLimit || *bindingLimit != jsonLimit {
						return nil, nil, fmt.Errorf(
							"令牌 %d 分组 %s 的稳定倍率保护与旧镜像不一致，请先修复数据后再迁移",
							token.Id,
							group.Code,
						)
					}
				}
			}
		}
		for code := range limits {
			if _, ok := selectedCodes[code]; !ok {
				return nil, nil, fmt.Errorf("令牌 %d 的倍率保护分组 %s 未在令牌分组中", token.Id, code)
			}
		}

		hasTarget := false
		for _, groupID := range groupIDs {
			if groupID == targetGroupID {
				hasTarget = true
				break
			}
		}
		if hasTarget {
			summary.DeduplicatedTokens++
			delete(limits, sourceGroup.Code)
		} else if sourceLimit, ok := limits[sourceGroup.Code]; ok {
			limits[targetGroup.Code] = sourceLimit
			delete(limits, sourceGroup.Code)
		}

		migratedIDs := make([]int, 0, len(groupIDs))
		seen := make(map[int]struct{}, len(groupIDs))
		for _, groupID := range groupIDs {
			if groupID == sourceGroupID {
				if hasTarget {
					continue
				}
				groupID = targetGroupID
			}
			if _, exists := seen[groupID]; exists {
				continue
			}
			seen[groupID] = struct{}{}
			migratedIDs = append(migratedIDs, groupID)
		}
		if len(migratedIDs) == 0 {
			return nil, nil, fmt.Errorf("令牌 %d 迁移后没有可用分组", token.Id)
		}

		migratedCodes := make([]string, 0, len(migratedIDs))
		migratedDetails := make([]GroupReference, 0, len(migratedIDs))
		migratedCodeSet := make(map[string]struct{}, len(migratedIDs))
		for _, groupID := range migratedIDs {
			group := groupsByID[groupID]
			migratedCodes = append(migratedCodes, group.Code)
			migratedDetails = append(migratedDetails, newGroupReference(group))
			migratedCodeSet[group.Code] = struct{}{}
		}
		for code := range limits {
			if _, ok := migratedCodeSet[code]; !ok {
				delete(limits, code)
			}
		}
		limitsJSON, err := marshalTokenGroupRatioLimitsForMigration(limits)
		if err != nil {
			return nil, nil, fmt.Errorf("序列化令牌 %d 的倍率保护失败: %w", token.Id, err)
		}

		if len(groupIDs) == 1 {
			summary.SingleGroupTokens++
		} else {
			summary.MultiGroupTokens++
		}
		if token.UserId > 0 {
			affectedUsers[token.UserId] = struct{}{}
		}
		token.Group = strings.Join(migratedCodes, ",")
		token.GroupMode = TokenGroupModeExplicit
		token.GroupIds = migratedIDs
		token.GroupDetails = migratedDetails
		token.GroupRatioLimits = limitsJSON
		plans = append(plans, tokenGroupMigrationPlan{token: *token})
		summary.MigratedTokens++
	}

	summary.AffectedUsers = len(affectedUsers)
	return plans, summary, nil
}

// PreviewTokenGroupMigration 返回迁移会影响的令牌数量，不修改数据。
func PreviewTokenGroupMigration(sourceGroupID, targetGroupID int) (*TokenGroupMigrationSummary, error) {
	if DB == nil {
		return nil, errors.New("database is nil")
	}
	_, summary, err := buildTokenGroupMigrationPlans(DB, sourceGroupID, targetGroupID, false)
	return summary, err
}

// MigrateTokenGroup 将所有明确绑定源分组的令牌原子迁移到目标分组。
func MigrateTokenGroup(sourceGroupID, targetGroupID int) (*TokenGroupMigrationSummary, error) {
	if DB == nil {
		return nil, errors.New("database is nil")
	}
	var plans []tokenGroupMigrationPlan
	var summary *TokenGroupMigrationSummary
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		plans, summary, err = buildTokenGroupMigrationPlans(tx, sourceGroupID, targetGroupID, true)
		if err != nil {
			return err
		}
		for index := range plans {
			token := &plans[index].token
			if err := tx.Unscoped().Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
				"group":              token.Group,
				"group_mode":         token.GroupMode,
				"group_ratio_limits": token.GroupRatioLimits,
			}).Error; err != nil {
				return fmt.Errorf("更新令牌 %d 分组镜像失败: %w", token.Id, err)
			}
			if err := replaceTokenGroupBindings(tx, token); err != nil {
				return fmt.Errorf("更新令牌 %d 分组绑定失败: %w", token.Id, err)
			}
		}

		var remainingBindings int64
		if err := tx.Model(&TokenGroupBinding{}).Where("group_id = ?", sourceGroupID).Count(&remainingBindings).Error; err != nil {
			return err
		}
		legacyIdentifiers, legacyIdentifierSet, err := groupLegacyIdentifiers(
			tx,
			&Group{Id: summary.SourceGroup.Id, Code: summary.SourceGroup.Code},
		)
		if err != nil {
			return err
		}
		var remainingTokens []Token
		if err := affectedTokensForGroupQuery(tx, sourceGroupID, legacyIdentifiers).
			Select("id", groupBindingGroupColumn()).
			Find(&remainingTokens).Error; err != nil {
			return err
		}
		var remainingLegacy int64
		for _, token := range remainingTokens {
			if containsLegacyGroupIdentifier(token.Group, legacyIdentifierSet) {
				remainingLegacy++
			}
		}
		if remainingBindings > 0 || remainingLegacy > 0 {
			return fmt.Errorf("源分组仍有 %d 条稳定绑定和 %d 条旧令牌引用，迁移已回滚", remainingBindings, remainingLegacy)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if common.RedisEnabled && len(plans) > 0 {
		for _, plan := range plans {
			if plan.token.DeletedAt.Valid || plan.token.Key == "" {
				continue
			}
			var invalidateErr error
			for attempt := 0; attempt < 3; attempt++ {
				invalidateErr = cacheDeleteToken(plan.token.Key)
				if invalidateErr == nil {
					break
				}
			}
			if invalidateErr != nil {
				summary.CacheInvalidationFailed++
				common.SysLog(fmt.Sprintf("failed to invalidate migrated token %d cache: %v", plan.token.Id, invalidateErr))
				continue
			}
			summary.CacheInvalidated++
		}
		if summary.CacheInvalidationFailed > 0 {
			summary.Warning = fmt.Sprintf(
				"数据库迁移已完成，但 %d 个令牌缓存清理失败，请在 Redis 恢复后清理令牌缓存",
				summary.CacheInvalidationFailed,
			)
		}
	}
	return summary, nil
}
