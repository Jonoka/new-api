package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GroupReference struct {
	Id int `json:"id"`
	// Code 仅供旧客户端兼容，新客户端应使用 Id 关联、使用 Name 展示。
	Code string `json:"code"`
	Name string `json:"name"`
	// Exclusive 供管理端编辑历史令牌时识别独立分组。
	Exclusive bool `json:"exclusive"`
}

func newGroupReference(group *Group) GroupReference {
	if group == nil {
		return GroupReference{}
	}
	return GroupReference{Id: group.Id, Code: group.Code, Name: group.Name, Exclusive: group.Exclusive}
}

var ErrTokenGroupBindingConflict = errors.New("当前绑定分组违规")

const exclusiveGroupSnapshotTTL = 5 * time.Second

var exclusiveGroupSnapshot = struct {
	sync.RWMutex
	db       *gorm.DB
	loadedAt time.Time
	ids      map[int]struct{}
}{}

// InvalidateExclusiveGroupSnapshot 使当前进程的独立分组快照立即失效。
func InvalidateExclusiveGroupSnapshot() {
	exclusiveGroupSnapshot.Lock()
	exclusiveGroupSnapshot.db = nil
	exclusiveGroupSnapshot.loadedAt = time.Time{}
	exclusiveGroupSnapshot.ids = nil
	exclusiveGroupSnapshot.Unlock()
}

func ensureExclusiveGroupSnapshot() error {
	if DB == nil {
		return errors.New("database is nil")
	}
	now := time.Now()
	exclusiveGroupSnapshot.RLock()
	fresh := exclusiveGroupSnapshot.db == DB &&
		exclusiveGroupSnapshot.ids != nil &&
		now.Sub(exclusiveGroupSnapshot.loadedAt) < exclusiveGroupSnapshotTTL
	exclusiveGroupSnapshot.RUnlock()
	if fresh {
		return nil
	}

	exclusiveGroupSnapshot.Lock()
	defer exclusiveGroupSnapshot.Unlock()
	now = time.Now()
	if exclusiveGroupSnapshot.db == DB &&
		exclusiveGroupSnapshot.ids != nil &&
		now.Sub(exclusiveGroupSnapshot.loadedAt) < exclusiveGroupSnapshotTTL {
		return nil
	}
	var ids []int
	if err := DB.Model(&Group{}).Where("exclusive = ?", true).Pluck("id", &ids).Error; err != nil {
		return err
	}
	snapshot := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			snapshot[id] = struct{}{}
		}
	}
	exclusiveGroupSnapshot.db = DB
	exclusiveGroupSnapshot.loadedAt = now
	exclusiveGroupSnapshot.ids = snapshot
	return nil
}

func uniquePositiveGroupIDs(ids []int) []int {
	seen := make(map[int]struct{}, len(ids))
	uniqueIDs := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	return uniqueIDs
}

// ValidateTokenExclusiveGroupBinding 保证独立分组只被单独绑定。
// 该检查每次从 groups 表读取当前属性，因此管理员修改分组后不依赖令牌缓存刷新。
func ValidateTokenExclusiveGroupBinding(tx *gorm.DB, token *Token) error {
	if tx == nil {
		return errors.New("database is nil")
	}
	if token == nil {
		return errors.New("token is nil")
	}
	if token.GroupMode == TokenGroupModeAuto || token.GroupMode == TokenGroupModeInherit {
		return nil
	}
	ids := append([]int(nil), token.GroupIds...)
	if len(ids) == 0 {
		legacyCodes := splitLegacyGroupCodes(token.Group)
		if len(legacyCodes) <= 1 {
			return nil
		}
		for _, code := range legacyCodes {
			group, err := GetGroupByCodeOrAliasWithDB(tx, code)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			ids = append(ids, group.Id)
		}
	}
	uniqueIDs := uniquePositiveGroupIDs(ids)
	if len(uniqueIDs) <= 1 {
		return nil
	}
	var count int64
	if err := tx.Model(&Group{}).
		Where("id IN ? AND exclusive = ?", uniqueIDs, true).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrTokenGroupBindingConflict
	}
	return nil
}

// ValidateTokenExclusiveGroupBindingCached 在鉴权热路径中使用短期快照，
// 避免每个多分组请求都访问 groups 表。写路径仍使用事务内强校验。
func ValidateTokenExclusiveGroupBindingCached(token *Token) error {
	if token == nil {
		return errors.New("token is nil")
	}
	if token.GroupMode == TokenGroupModeAuto || token.GroupMode == TokenGroupModeInherit {
		return nil
	}
	ids := append([]int(nil), token.GroupIds...)
	if len(ids) == 0 {
		for _, detail := range token.GroupDetails {
			ids = append(ids, detail.Id)
		}
	}
	uniqueIDs := uniquePositiveGroupIDs(ids)
	if len(uniqueIDs) <= 1 {
		return nil
	}
	if err := ensureExclusiveGroupSnapshot(); err != nil {
		return err
	}
	exclusiveGroupSnapshot.RLock()
	defer exclusiveGroupSnapshot.RUnlock()
	for _, id := range uniqueIDs {
		if _, exclusive := exclusiveGroupSnapshot.ids[id]; exclusive {
			return ErrTokenGroupBindingConflict
		}
	}
	return nil
}

// ChannelGroupBinding 通过 GroupId 关联渠道与分组；Position 保留管理端选择顺序。
type ChannelGroupBinding struct {
	ChannelId int `json:"channel_id" gorm:"primaryKey;autoIncrement:false;uniqueIndex:idx_channel_group_position,priority:1"`
	GroupId   int `json:"group_id" gorm:"primaryKey;autoIncrement:false;index"`
	Position  int `json:"position" gorm:"not null;uniqueIndex:idx_channel_group_position,priority:2"`
}

func (ChannelGroupBinding) TableName() string { return "channel_groups" }

// TokenGroupBinding 保存显式令牌分组的顺序和每组倍率保护。
type TokenGroupBinding struct {
	TokenId    int      `json:"token_id" gorm:"primaryKey;autoIncrement:false;uniqueIndex:idx_token_group_position,priority:1"`
	GroupId    int      `json:"group_id" gorm:"primaryKey;autoIncrement:false;index"`
	Position   int      `json:"position" gorm:"not null;uniqueIndex:idx_token_group_position,priority:2"`
	RatioLimit *float64 `json:"ratio_limit,omitempty"`
}

func (TokenGroupBinding) TableName() string { return "token_groups" }

const (
	TokenGroupModeInherit  = "inherit"
	TokenGroupModeExplicit = "explicit"
	TokenGroupModeAuto     = "auto"
)

func splitLegacyGroupCodes(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

type invalidLegacyGroupCodeError struct {
	code  string
	cause error
}

func (err *invalidLegacyGroupCodeError) Error() string {
	return fmt.Sprintf("历史分组标识 %q 无效: %v", legacyGroupCodePreview(err.code), err.cause)
}

func (err *invalidLegacyGroupCodeError) Unwrap() error {
	return err.cause
}

func legacyGroupCodePreview(code string) string {
	const maxRunes = 64
	runes := []rune(strings.TrimSpace(code))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func isInvalidLegacyGroupCodeError(err error) bool {
	var target *invalidLegacyGroupCodeError
	return errors.As(err, &target)
}

func groupBindingTablesReady(tx *gorm.DB, table interface{}) bool {
	return tx != nil && tx.Migrator().HasTable(&Group{}) && tx.Migrator().HasTable(table)
}

func groupBindingGroupColumn() string {
	if common.UsingPostgreSQL {
		return `"group"`
	}
	return "`group`"
}

type groupBindingResolvePolicy struct {
	allowedDisabledIDs map[int]struct{}
	allowAllDisabled   bool
	tablesVerified     bool
}

func (policy groupBindingResolvePolicy) allows(group *Group) bool {
	if group == nil || group.Status == GroupStatusActive {
		return group != nil
	}
	if policy.allowAllDisabled {
		return true
	}
	_, ok := policy.allowedDisabledIDs[group.Id]
	return ok
}

func getGroupsByIDsWithDB(tx *gorm.DB, ids []int) (map[int]*Group, error) {
	result := make(map[int]*Group, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var groups []*Group
	if err := tx.Where("id IN ?", ids).Find(&groups).Error; err != nil {
		return nil, err
	}
	for _, group := range groups {
		result[group.Id] = group
	}
	return result, nil
}

func getGroupByCodeOrAliasStrict(tx *gorm.DB, code string) (*Group, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var group Group
	if err := tx.Where("code = ?", code).First(&group).Error; err == nil {
		return &group, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var alias GroupAlias
	if err := tx.Where("alias = ?", code).First(&alias).Error; err != nil {
		return nil, err
	}
	if err := tx.First(&group, "id = ?", alias.GroupId).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func resolveBindingGroupsWithPolicy(tx *gorm.DB, ids []int, legacy string, policy groupBindingResolvePolicy) ([]int, []string, []GroupReference, error) {
	if tx == nil || (!policy.tablesVerified && !tx.Migrator().HasTable(&Group{})) {
		return nil, splitLegacyGroupCodes(legacy), nil, nil
	}
	orderedIDs := make([]int, 0)
	orderedCodes := make([]string, 0)
	references := make([]GroupReference, 0)
	seen := make(map[int]struct{})

	if len(ids) > 0 {
		groups, err := getGroupsByIDsWithDB(tx, ids)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			group, ok := groups[id]
			if !ok {
				return nil, nil, nil, fmt.Errorf("分组 ID %d 不存在", id)
			}
			if !policy.allows(group) {
				return nil, nil, nil, fmt.Errorf("分组 ID %d 不存在或已禁用", id)
			}
			seen[id] = struct{}{}
			orderedIDs = append(orderedIDs, id)
			orderedCodes = append(orderedCodes, group.Code)
			references = append(references, newGroupReference(group))
		}
		return orderedIDs, orderedCodes, references, nil
	}

	legacyCodes := splitLegacyGroupCodes(legacy)
	if len(legacyCodes) == 0 && strings.TrimSpace(legacy) != "" {
		return nil, nil, nil, &invalidLegacyGroupCodeError{code: legacy, cause: errors.New("分组标识不能为空")}
	}
	for _, code := range legacyCodes {
		if _, err := NormalizeGroupCode(code); err != nil {
			return nil, nil, nil, &invalidLegacyGroupCodeError{code: code, cause: err}
		}
		var group *Group
		var err error
		if policy.tablesVerified {
			group, err = getGroupByCodeOrAliasStrict(tx, code)
		} else {
			group, err = GetGroupByCodeOrAliasWithDB(tx, code)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, &invalidLegacyGroupCodeError{
				code:  code,
				cause: fmt.Errorf("对应分组不存在: %w", err),
			}
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("查询分组 %q 失败: %w", legacyGroupCodePreview(code), err)
		}
		if !policy.allows(group) {
			return nil, nil, nil, fmt.Errorf("分组 %q 已禁用", code)
		}
		if _, ok := seen[group.Id]; ok {
			continue
		}
		seen[group.Id] = struct{}{}
		orderedIDs = append(orderedIDs, group.Id)
		orderedCodes = append(orderedCodes, group.Code)
		references = append(references, newGroupReference(group))
	}
	return orderedIDs, orderedCodes, references, nil
}

func resolveBindingGroups(tx *gorm.DB, ids []int, legacy string) ([]int, []string, []GroupReference, error) {
	return resolveBindingGroupsWithPolicy(tx, ids, legacy, groupBindingResolvePolicy{})
}

func loadChannelBindingIDs(tx *gorm.DB, channelID int) ([]int, error) {
	if tx == nil || channelID <= 0 || !tx.Migrator().HasTable(&ChannelGroupBinding{}) {
		return nil, nil
	}
	var ids []int
	err := tx.Model(&ChannelGroupBinding{}).
		Where("channel_id = ?", channelID).
		Order("position ASC").
		Pluck("group_id", &ids).Error
	return ids, err
}

func loadTokenBindingIDs(tx *gorm.DB, tokenID int) ([]int, error) {
	if tx == nil || tokenID <= 0 || !tx.Migrator().HasTable(&TokenGroupBinding{}) {
		return nil, nil
	}
	var ids []int
	err := tx.Model(&TokenGroupBinding{}).
		Where("token_id = ?", tokenID).
		Order("position ASC").
		Pluck("group_id", &ids).Error
	return ids, err
}

func addLegacyGroupIDs(tx *gorm.DB, ids map[int]struct{}, legacy string) {
	if tx == nil || !tx.Migrator().HasTable(&Group{}) {
		return
	}
	for _, code := range splitLegacyGroupCodes(legacy) {
		group, err := GetGroupByCodeOrAliasWithDB(tx, code)
		if err == nil {
			ids[group.Id] = struct{}{}
		}
	}
}

func existingChannelGroupIDs(tx *gorm.DB, channelID int) (map[int]struct{}, error) {
	result := make(map[int]struct{})
	ids, err := loadChannelBindingIDs(tx, channelID)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		result[id] = struct{}{}
	}
	if len(result) > 0 || tx == nil || channelID <= 0 || !tx.Migrator().HasTable(&Channel{}) {
		return result, nil
	}
	var channel Channel
	if err := tx.Select("id", groupBindingGroupColumn()).First(&channel, "id = ?", channelID).Error; err != nil {
		return nil, err
	}
	addLegacyGroupIDs(tx, result, channel.Group)
	return result, nil
}

func existingTokenGroupIDs(tx *gorm.DB, tokenID int) (map[int]struct{}, error) {
	result := make(map[int]struct{})
	ids, err := loadTokenBindingIDs(tx, tokenID)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		result[id] = struct{}{}
	}
	if len(result) > 0 || tx == nil || tokenID <= 0 || !tx.Migrator().HasTable(&Token{}) {
		return result, nil
	}
	var token Token
	if err := tx.Select("id", groupBindingGroupColumn()).First(&token, "id = ?", tokenID).Error; err != nil {
		return nil, err
	}
	addLegacyGroupIDs(tx, result, token.Group)
	return result, nil
}

func prepareChannelGroupBindings(tx *gorm.DB, channel *Channel, policy groupBindingResolvePolicy) error {
	if channel == nil {
		return errors.New("channel is nil")
	}
	if channel.GroupIds != nil && len(channel.GroupIds) == 0 {
		return errors.New("渠道分组不能为空")
	}
	ids, codes, details, err := resolveBindingGroupsWithPolicy(tx, channel.GroupIds, channel.Group, policy)
	if err != nil {
		return err
	}
	channel.GroupIds = ids
	channel.GroupDetails = details
	if len(codes) > 0 {
		channel.Group = strings.Join(codes, ",")
	}
	return nil
}

func PrepareChannelGroupBindings(tx *gorm.DB, channel *Channel) error {
	return prepareChannelGroupBindings(tx, channel, groupBindingResolvePolicy{})
}

func PrepareChannelGroupBindingsForUpdate(tx *gorm.DB, channel *Channel) error {
	if channel == nil {
		return errors.New("channel is nil")
	}
	allowedDisabledIDs, err := existingChannelGroupIDs(tx, channel.Id)
	if err != nil {
		return err
	}
	return prepareChannelGroupBindings(tx, channel, groupBindingResolvePolicy{allowedDisabledIDs: allowedDisabledIDs})
}

func buildChannelGroupBindings(channel *Channel) []ChannelGroupBinding {
	bindings := make([]ChannelGroupBinding, 0, len(channel.GroupIds))
	for position, groupID := range channel.GroupIds {
		bindings = append(bindings, ChannelGroupBinding{ChannelId: channel.Id, GroupId: groupID, Position: position})
	}
	return bindings
}

func replaceChannelGroupBindings(tx *gorm.DB, channel *Channel) error {
	if tx == nil {
		return errors.New("database is nil")
	}
	if channel == nil || channel.Id <= 0 {
		return nil
	}
	if err := tx.Where("channel_id = ?", channel.Id).Delete(&ChannelGroupBinding{}).Error; err != nil {
		return err
	}
	bindings := buildChannelGroupBindings(channel)
	if len(bindings) == 0 {
		return nil
	}
	return tx.Create(&bindings).Error
}

func insertChannelGroupBindingsForBackfill(tx *gorm.DB, channel *Channel) error {
	if tx == nil {
		return errors.New("database is nil")
	}
	if channel == nil || channel.Id <= 0 {
		return nil
	}
	bindings := buildChannelGroupBindings(channel)
	if len(bindings) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&bindings).Error
}

func writeChannelGroupBindings(tx *gorm.DB, channel *Channel) error {
	if channel == nil || channel.Id <= 0 || !groupBindingTablesReady(tx, &ChannelGroupBinding{}) {
		return nil
	}
	if err := lockChannelGroupBindingGroups(tx, channel); err != nil {
		return err
	}
	return replaceChannelGroupBindings(tx, channel)
}

func ReplaceChannelGroupBindings(tx *gorm.DB, channel *Channel) error {
	if err := PrepareChannelGroupBindings(tx, channel); err != nil {
		return err
	}
	return writeChannelGroupBindings(tx, channel)
}

// ReplaceChannelGroupBindingsForUpdate 保留当前渠道已经使用的禁用分组，
// 同时仍拒绝把新的禁用分组绑定到渠道。
func ReplaceChannelGroupBindingsForUpdate(tx *gorm.DB, channel *Channel) error {
	if channel == nil {
		return errors.New("channel is nil")
	}
	if err := PrepareChannelGroupBindingsForUpdate(tx, channel); err != nil {
		return err
	}
	return writeChannelGroupBindings(tx, channel)
}

func HydrateChannelGroupBindings(tx *gorm.DB, channels []*Channel) error {
	if len(channels) == 0 || !groupBindingTablesReady(tx, &ChannelGroupBinding{}) {
		return nil
	}
	channelIDs := make([]int, 0, len(channels))
	byID := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		if channel == nil || channel.Id <= 0 {
			continue
		}
		channelIDs = append(channelIDs, channel.Id)
		byID[channel.Id] = channel
	}
	if len(channelIDs) == 0 {
		return nil
	}
	var bindings []ChannelGroupBinding
	if err := tx.Where("channel_id IN ?", channelIDs).Order("channel_id ASC, position ASC").Find(&bindings).Error; err != nil {
		return err
	}
	groupIDs := make([]int, 0, len(bindings))
	for _, binding := range bindings {
		groupIDs = append(groupIDs, binding.GroupId)
	}
	groups, err := getGroupsByIDsWithDB(tx, groupIDs)
	if err != nil {
		return err
	}
	hasBindings := make(map[int]bool, len(bindings))
	clearedChannels := make(map[int]bool, len(bindings))
	for _, binding := range bindings {
		hasBindings[binding.ChannelId] = true
		channel := byID[binding.ChannelId]
		group := groups[binding.GroupId]
		if channel == nil || group == nil {
			continue
		}
		if !clearedChannels[binding.ChannelId] {
			channel.GroupIds = nil
			channel.GroupDetails = nil
			clearedChannels[binding.ChannelId] = true
		}
		channel.GroupIds = append(channel.GroupIds, group.Id)
		channel.GroupDetails = append(channel.GroupDetails, newGroupReference(group))
	}
	// 尚未回填关联的旧记录也提供结构化投影。
	for _, channel := range channels {
		if channel == nil || hasBindings[channel.Id] {
			continue
		}
		if strings.TrimSpace(channel.Group) == "" {
			channel.GroupIds = nil
			channel.GroupDetails = nil
			continue
		}
		ids, _, details, resolveErr := resolveBindingGroupsWithPolicy(
			tx,
			nil,
			channel.Group,
			groupBindingResolvePolicy{allowAllDisabled: true, tablesVerified: true},
		)
		if resolveErr != nil {
			if isInvalidLegacyGroupCodeError(resolveErr) {
				continue
			}
			return fmt.Errorf("加载渠道 %d 分组关联失败: %w", channel.Id, resolveErr)
		}
		channel.GroupIds = ids
		channel.GroupDetails = details
	}
	return nil
}

func inferStoredTokenGroupMode(token *Token) string {
	if token == nil {
		return TokenGroupModeInherit
	}
	if strings.EqualFold(strings.TrimSpace(token.Group), "auto") || token.GroupMode == TokenGroupModeAuto {
		return TokenGroupModeAuto
	}
	if len(token.GroupIds) > 0 || strings.TrimSpace(token.Group) != "" || token.GroupMode == TokenGroupModeExplicit {
		return TokenGroupModeExplicit
	}
	return TokenGroupModeInherit
}

func inferTokenGroupModeForWrite(token *Token) string {
	if token == nil {
		return TokenGroupModeInherit
	}
	mode := strings.ToLower(strings.TrimSpace(token.GroupMode))
	switch mode {
	case TokenGroupModeInherit, TokenGroupModeExplicit, TokenGroupModeAuto:
		return mode
	case "":
	default:
		return mode
	}
	if token.GroupIds != nil {
		if len(token.GroupIds) == 0 {
			return TokenGroupModeInherit
		}
		return TokenGroupModeExplicit
	}
	if strings.EqualFold(strings.TrimSpace(token.Group), "auto") {
		return TokenGroupModeAuto
	}
	if strings.TrimSpace(token.Group) != "" {
		return TokenGroupModeExplicit
	}
	return TokenGroupModeInherit
}

func prepareTokenGroupBindings(tx *gorm.DB, token *Token, policy groupBindingResolvePolicy) error {
	if token == nil {
		return errors.New("token is nil")
	}
	token.GroupMode = inferTokenGroupModeForWrite(token)
	switch token.GroupMode {
	case TokenGroupModeInherit:
		token.Group = ""
		token.GroupIds = nil
		token.GroupDetails = nil
		return nil
	case TokenGroupModeAuto:
		token.Group = "auto"
		token.GroupIds = nil
		token.GroupDetails = nil
		return nil
	case TokenGroupModeExplicit:
		if token.GroupIds != nil && len(token.GroupIds) == 0 {
			return errors.New("显式令牌分组不能为空")
		}
		ids, codes, details, err := resolveBindingGroupsWithPolicy(tx, token.GroupIds, token.Group, policy)
		if err != nil {
			return err
		}
		if len(ids) == 0 && tx != nil && (policy.tablesVerified || tx.Migrator().HasTable(&Group{})) {
			return &invalidLegacyGroupCodeError{code: token.Group, cause: errors.New("显式令牌分组不能为空")}
		}
		token.GroupIds = ids
		token.GroupDetails = details
		token.Group = strings.Join(codes, ",")
		return nil
	default:
		return fmt.Errorf("不支持的令牌分组模式: %s", token.GroupMode)
	}
}

func PrepareTokenGroupBindings(tx *gorm.DB, token *Token) error {
	return prepareTokenGroupBindings(tx, token, groupBindingResolvePolicy{})
}

func PrepareTokenGroupBindingsForUpdate(tx *gorm.DB, token *Token) error {
	if token == nil {
		return errors.New("token is nil")
	}
	allowedDisabledIDs, err := existingTokenGroupIDs(tx, token.Id)
	if err != nil {
		return err
	}
	return prepareTokenGroupBindings(tx, token, groupBindingResolvePolicy{allowedDisabledIDs: allowedDisabledIDs})
}

func buildTokenGroupBindings(token *Token) []TokenGroupBinding {
	limits := token.GetGroupRatioLimitsMap()
	bindings := make([]TokenGroupBinding, 0, len(token.GroupIds))
	for position, groupID := range token.GroupIds {
		binding := TokenGroupBinding{TokenId: token.Id, GroupId: groupID, Position: position}
		if position < len(token.GroupDetails) {
			if limit, ok := limits[token.GroupDetails[position].Code]; ok {
				limitCopy := limit
				binding.RatioLimit = &limitCopy
			}
		}
		bindings = append(bindings, binding)
	}
	return bindings
}

func replaceTokenGroupBindings(tx *gorm.DB, token *Token) error {
	if tx == nil {
		return errors.New("database is nil")
	}
	if token == nil || token.Id <= 0 {
		return nil
	}
	if err := tx.Where("token_id = ?", token.Id).Delete(&TokenGroupBinding{}).Error; err != nil {
		return err
	}
	if token.GroupMode != TokenGroupModeExplicit {
		return nil
	}
	bindings := buildTokenGroupBindings(token)
	if len(bindings) == 0 {
		return nil
	}
	return tx.Create(&bindings).Error
}

func insertTokenGroupBindingsForBackfill(tx *gorm.DB, token *Token) error {
	if tx == nil {
		return errors.New("database is nil")
	}
	if token == nil || token.Id <= 0 || token.GroupMode != TokenGroupModeExplicit {
		return nil
	}
	bindings := buildTokenGroupBindings(token)
	if len(bindings) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&bindings).Error
}

func lockGroupRowsForBindingWrite(tx *gorm.DB, groupIDs []int, owner string) error {
	if tx == nil {
		return errors.New("database is nil")
	}
	if len(groupIDs) == 0 {
		return nil
	}
	ids := append([]int(nil), groupIDs...)
	sort.Ints(ids)
	uniqueIDs := ids[:0]
	for _, id := range ids {
		if id <= 0 || (len(uniqueIDs) > 0 && uniqueIDs[len(uniqueIDs)-1] == id) {
			continue
		}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return fmt.Errorf("%s分组不能为空", owner)
	}
	var lockedGroups []Group
	if err := lockForUpdate(tx.Model(&Group{})).
		Select("id").
		Where("id IN ?", uniqueIDs).
		Order("id ASC").
		Find(&lockedGroups).Error; err != nil {
		return err
	}
	if len(lockedGroups) != len(uniqueIDs) {
		return fmt.Errorf("%s分组在写入期间已被删除，请重试", owner)
	}
	return nil
}

func lockChannelGroupBindingGroups(tx *gorm.DB, channel *Channel) error {
	if channel == nil || len(channel.GroupIds) == 0 {
		return nil
	}
	return lockGroupRowsForBindingWrite(tx, channel.GroupIds, "渠道")
}

func lockTokenGroupBindingGroups(tx *gorm.DB, token *Token) error {
	if token == nil || token.GroupMode != TokenGroupModeExplicit || len(token.GroupIds) == 0 {
		return nil
	}
	return lockGroupRowsForBindingWrite(tx, token.GroupIds, "令牌")
}

func writeTokenGroupBindings(tx *gorm.DB, token *Token) error {
	if token == nil || token.Id <= 0 || !groupBindingTablesReady(tx, &TokenGroupBinding{}) {
		return nil
	}
	if err := lockTokenGroupBindingGroups(tx, token); err != nil {
		return err
	}
	return replaceTokenGroupBindings(tx, token)
}

func ReplaceTokenGroupBindings(tx *gorm.DB, token *Token) error {
	if err := PrepareTokenGroupBindings(tx, token); err != nil {
		return err
	}
	return writeTokenGroupBindings(tx, token)
}

func HydrateTokenGroupBindings(tx *gorm.DB, tokens []*Token) error {
	if len(tokens) == 0 || !groupBindingTablesReady(tx, &TokenGroupBinding{}) {
		return nil
	}
	tokenIDs := make([]int, 0, len(tokens))
	byID := make(map[int]*Token, len(tokens))
	for _, token := range tokens {
		if token == nil || token.Id <= 0 {
			continue
		}
		tokenIDs = append(tokenIDs, token.Id)
		byID[token.Id] = token
	}
	if len(tokenIDs) == 0 {
		return nil
	}
	var bindings []TokenGroupBinding
	if err := tx.Where("token_id IN ?", tokenIDs).Order("token_id ASC, position ASC").Find(&bindings).Error; err != nil {
		return err
	}
	groupIDs := make([]int, 0, len(bindings))
	for _, binding := range bindings {
		groupIDs = append(groupIDs, binding.GroupId)
	}
	groups, err := getGroupsByIDsWithDB(tx, groupIDs)
	if err != nil {
		return err
	}
	hasBindings := make(map[int]bool, len(bindings))
	clearedTokens := make(map[int]bool, len(bindings))
	for _, binding := range bindings {
		hasBindings[binding.TokenId] = true
		token := byID[binding.TokenId]
		group := groups[binding.GroupId]
		if token == nil || group == nil {
			continue
		}
		if !clearedTokens[binding.TokenId] {
			token.GroupIds = nil
			token.GroupDetails = nil
			clearedTokens[binding.TokenId] = true
		}
		token.GroupMode = TokenGroupModeExplicit
		token.GroupIds = append(token.GroupIds, group.Id)
		token.GroupDetails = append(token.GroupDetails, newGroupReference(group))
	}
	for _, token := range tokens {
		if token == nil || hasBindings[token.Id] {
			continue
		}
		token.GroupMode = inferStoredTokenGroupMode(token)
		switch token.GroupMode {
		case TokenGroupModeAuto:
			token.Group = "auto"
			token.GroupIds = nil
			token.GroupDetails = nil
		case TokenGroupModeExplicit:
			ids, _, details, resolveErr := resolveBindingGroupsWithPolicy(
				tx,
				nil,
				token.Group,
				groupBindingResolvePolicy{allowAllDisabled: true, tablesVerified: true},
			)
			if resolveErr != nil {
				if isInvalidLegacyGroupCodeError(resolveErr) {
					continue
				}
				return fmt.Errorf("加载令牌 %d 分组关联失败: %w", token.Id, resolveErr)
			}
			token.GroupIds = ids
			token.GroupDetails = details
		default:
			token.Group = ""
			token.GroupIds = nil
			token.GroupDetails = nil
		}
	}
	return nil
}

// BackfillGroupBindings 在只增不删的迁移阶段建立关联表和令牌模式。
func BackfillGroupBindings() error {
	if DB == nil {
		return errors.New("database is nil")
	}
	type skippedBinding struct {
		entity string
		id     int
		reason string
	}
	const maxSkippedBindingSamples = 10
	skippedCount := 0
	skipped := make([]skippedBinding, 0, maxSkippedBindingSamples)
	recordSkipped := func(entity string, id int, err error) {
		skippedCount++
		if len(skipped) < maxSkippedBindingSamples {
			skipped = append(skipped, skippedBinding{entity: entity, id: id, reason: err.Error()})
		}
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var groupProbe []Group
		if err := tx.Select("id", "code", "status").Limit(1).Find(&groupProbe).Error; err != nil {
			return fmt.Errorf("检查分组表失败: %w", err)
		}
		var aliasProbe []GroupAlias
		if err := tx.Select("id", "alias", "group_id").Limit(1).Find(&aliasProbe).Error; err != nil {
			return fmt.Errorf("检查分组别名表失败: %w", err)
		}

		var channelBindings []ChannelGroupBinding
		if err := tx.Select("channel_id", "group_id", "position").Find(&channelBindings).Error; err != nil {
			return fmt.Errorf("加载现有渠道分组关联失败: %w", err)
		}
		channelsWithBindings := make(map[int]struct{}, len(channelBindings))
		for _, binding := range channelBindings {
			channelsWithBindings[binding.ChannelId] = struct{}{}
		}

		var tokenBindings []TokenGroupBinding
		if err := tx.Select("token_id", "group_id", "position", "ratio_limit").Find(&tokenBindings).Error; err != nil {
			return fmt.Errorf("加载现有令牌分组关联失败: %w", err)
		}
		tokensWithBindings := make(map[int]struct{}, len(tokenBindings))
		for _, binding := range tokenBindings {
			tokensWithBindings[binding.TokenId] = struct{}{}
		}

		var channels []*Channel
		if err := tx.Select("id", groupBindingGroupColumn()).Find(&channels).Error; err != nil {
			return fmt.Errorf("加载待回填渠道失败: %w", err)
		}
		for _, channel := range channels {
			if _, ok := channelsWithBindings[channel.Id]; ok {
				continue
			}
			if err := prepareChannelGroupBindings(tx, channel, groupBindingResolvePolicy{allowAllDisabled: true, tablesVerified: true}); err != nil {
				if isInvalidLegacyGroupCodeError(err) {
					recordSkipped("渠道", channel.Id, err)
					continue
				}
				return fmt.Errorf("回填渠道 %d 分组失败: %w", channel.Id, err)
			}
			if err := insertChannelGroupBindingsForBackfill(tx, channel); err != nil {
				return fmt.Errorf("回填渠道 %d 分组失败: %w", channel.Id, err)
			}
		}
		var tokens []*Token
		if err := tx.Select("id", groupBindingGroupColumn(), "group_mode", "group_ratio_limits").Find(&tokens).Error; err != nil {
			return fmt.Errorf("加载待回填令牌失败: %w", err)
		}
		for _, token := range tokens {
			if _, ok := tokensWithBindings[token.Id]; ok {
				if token.GroupMode != TokenGroupModeExplicit {
					if err := tx.Model(&Token{}).Where("id = ?", token.Id).Update("group_mode", TokenGroupModeExplicit).Error; err != nil {
						return err
					}
				}
				continue
			}
			token.GroupMode = inferStoredTokenGroupMode(token)
			if err := prepareTokenGroupBindings(tx, token, groupBindingResolvePolicy{allowAllDisabled: true, tablesVerified: true}); err != nil {
				if isInvalidLegacyGroupCodeError(err) {
					recordSkipped("令牌", token.Id, err)
					continue
				}
				return fmt.Errorf("回填令牌 %d 分组失败: %w", token.Id, err)
			}
			if err := tx.Model(&Token{}).Where("id = ?", token.Id).Update("group_mode", token.GroupMode).Error; err != nil {
				return err
			}
			if err := insertTokenGroupBindingsForBackfill(tx, token); err != nil {
				return fmt.Errorf("回填令牌 %d 分组失败: %w", token.Id, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if skippedCount > 0 {
		samples := make([]string, 0, len(skipped))
		for _, item := range skipped {
			samples = append(samples, fmt.Sprintf("%s %d（%s）", item.entity, item.id, item.reason))
		}
		remaining := ""
		if skippedCount > len(skipped) {
			remaining = fmt.Sprintf("；另有 %d 条未展示", skippedCount-len(skipped))
		}
		common.SysError(fmt.Sprintf("警告：关联回填跳过 %d 条历史非法分组记录，原值均已保留供管理员修复；样本：%s%s", skippedCount, strings.Join(samples, "；"), remaining))
	}
	return nil
}

func deleteChannelGroupBindings(tx *gorm.DB, channelIDs []int) error {
	if len(channelIDs) == 0 || !tx.Migrator().HasTable(&ChannelGroupBinding{}) {
		return nil
	}
	return tx.Where("channel_id IN ?", channelIDs).Delete(&ChannelGroupBinding{}).Error
}

func deleteTokenGroupBindings(tx *gorm.DB, tokenIDs []int) error {
	if len(tokenIDs) == 0 || !tx.Migrator().HasTable(&TokenGroupBinding{}) {
		return nil
	}
	return tx.Where("token_id IN ?", tokenIDs).Delete(&TokenGroupBinding{}).Error
}
