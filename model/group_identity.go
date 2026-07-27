package model

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Group 使用 Id 作为唯一稳定身份。
//
// Code 仅保留给旧 API、旧配置和迁移期运行时使用，不应作为第二个名称展示；
// Name 是当前分组名称。所有新的持久化关联都必须使用 Group.Id。
type Group struct {
	Id             int     `json:"id"`
	Code           string  `json:"code" gorm:"size:64;not null;uniqueIndex:idx_groups_code"`
	Name           string  `json:"name" gorm:"size:128;not null;uniqueIndex:idx_groups_name"`
	Description    string  `json:"description,omitempty" gorm:"type:text"`
	Ratio          float64 `json:"ratio" gorm:"default:1"`
	UserSelectable bool    `json:"user_selectable" gorm:"default:false"`
	Exclusive      bool    `json:"exclusive" gorm:"default:false;index"`
	Status         int     `json:"status" gorm:"default:1;index"`
	CreatedTime    int64   `json:"created_time" gorm:"bigint"`
	UpdatedTime    int64   `json:"updated_time" gorm:"bigint"`

	// AutoEnabled/AutoOrder 是自动分组关联表的 API 投影，不直接存储在 groups 表。
	AutoEnabled bool `json:"auto_enabled" gorm:"-"`
	AutoOrder   int  `json:"auto_order" gorm:"-"`
}

func (Group) TableName() string { return "groups" }

// GroupAlias 把旧字符串输入解析到稳定 Group.Id，仅用于兼容历史客户端和数据。
type GroupAlias struct {
	Id        int    `json:"id"`
	Alias     string `json:"alias" gorm:"size:64;not null;uniqueIndex:idx_group_aliases_alias"`
	GroupId   int    `json:"group_id" gorm:"not null;index"`
	CreatedAt int64  `json:"created_at" gorm:"bigint"`
}

func (GroupAlias) TableName() string { return "group_aliases" }

// AutoGroupMember 保存自动分组的顺序。auto 本身是令牌选择模式，不是一个 Group。
type AutoGroupMember struct {
	GroupId  int `json:"group_id" gorm:"primaryKey;index"`
	Position int `json:"position" gorm:"not null;uniqueIndex:idx_auto_group_position"`
}

func (AutoGroupMember) TableName() string { return "auto_group_members" }

const (
	GroupStatusDisabled = 0
	GroupStatusActive   = 1
)

func isVirtualAutoCode(code string) bool {
	return strings.EqualFold(strings.TrimSpace(code), TokenGroupModeAuto)
}

var reservedGroupCodes = map[string]struct{}{
	"":     {},
	"auto": {},
	"all":  {},
	"null": {},
}

// NormalizeGroupCode 只规范化首尾空白，保持旧系统的大小写语义。
func NormalizeGroupCode(code string) (string, error) {
	code = strings.TrimSpace(code)
	if _, reserved := reservedGroupCodes[strings.ToLower(code)]; reserved {
		return "", fmt.Errorf("分组标识 %q 是保留值", code)
	}
	if strings.Contains(code, ",") {
		return "", errors.New("分组标识不能包含逗号")
	}
	if utf8.RuneCountInString(code) > 64 {
		return "", errors.New("分组标识长度不能超过 64 个字符")
	}
	return code, nil
}

func normalizeGroupName(name, fallback string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fallback
	}
	if name == "" {
		return "", errors.New("分组名称不能为空")
	}
	if utf8.RuneCountInString(name) > 128 {
		return "", errors.New("分组名称长度不能超过 128 个字符")
	}
	return name, nil
}

// GroupConfig 是分组管理页面的结构化保存格式。
type GroupConfig struct {
	Id             int     `json:"id"`
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Ratio          float64 `json:"ratio"`
	UserSelectable bool    `json:"user_selectable"`
	Exclusive      bool    `json:"exclusive"`
	// ExclusiveOmitted 仅用于保存请求；旧客户端缺失 exclusive 时保留数据库现值。
	ExclusiveOmitted bool `json:"-"`
	Status           int  `json:"status"`
	AutoEnabled      bool `json:"auto_enabled"`
	AutoOrder        int  `json:"auto_order"`
}

type GroupConfigSaveResult struct {
	MigratedTokens          int    `json:"migrated_tokens"`
	CleanedDeletedTokens    int    `json:"cleaned_deleted_tokens"`
	CacheInvalidated        int    `json:"cache_invalidated"`
	CacheInvalidationFailed int    `json:"cache_invalidation_failed"`
	Warning                 string `json:"warning,omitempty"`
}

func (g *Group) ToConfig(autoMembers map[int]AutoGroupMember) GroupConfig {
	config := GroupConfig{
		Id:             g.Id,
		Code:           g.Code,
		Name:           g.Name,
		Description:    g.Description,
		Ratio:          g.Ratio,
		UserSelectable: g.UserSelectable,
		Exclusive:      g.Exclusive,
		Status:         g.Status,
	}
	if member, ok := autoMembers[g.Id]; ok {
		config.AutoEnabled = true
		config.AutoOrder = member.Position
	}
	return config
}

func getAutoGroupMembers(tx *gorm.DB) (map[int]AutoGroupMember, error) {
	var members []AutoGroupMember
	if err := tx.Order("position ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	result := make(map[int]AutoGroupMember, len(members))
	for _, member := range members {
		result[member.GroupId] = member
	}
	return result, nil
}

// GetAllGroups 返回结构化分组列表。默认只返回启用分组，管理端可请求全部。
func GetAllGroups(includeDisabled bool) ([]*Group, error) {
	query := DB.Model(&Group{}).
		Where("LOWER(code) <> ?", TokenGroupModeAuto).
		Order("id ASC")
	if !includeDisabled {
		query = query.Where("status = ?", GroupStatusActive)
	}
	var groups []*Group
	if err := query.Find(&groups).Error; err != nil {
		return nil, err
	}
	members, err := getAutoGroupMembers(DB)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if member, ok := members[group.Id]; ok {
			group.AutoEnabled = true
			group.AutoOrder = member.Position
		}
	}
	return groups, nil
}

// GetActiveGroupNameMap 返回内部兼容标识到当前显示名称的映射。
//
// 业务逻辑仍使用 Code 做筛选和计费；面向用户的页面只使用 Name 展示，
// 因此管理员修改分组名称后无需重绑渠道、令牌或模型可用分组。
func GetActiveGroupNameMap() (map[string]string, error) {
	var groups []Group
	if err := DB.Model(&Group{}).
		Select("code", "name").
		Where("status = ?", GroupStatusActive).
		Find(&groups).Error; err != nil {
		return nil, err
	}

	result := make(map[string]string, len(groups))
	for _, group := range groups {
		code := strings.TrimSpace(group.Code)
		if code == "" || isVirtualAutoCode(code) {
			continue
		}
		name := strings.TrimSpace(group.Name)
		if name == "" {
			name = code
		}
		result[code] = name
	}
	return result, nil
}

// GetGroupDisplayNameMap 返回历史字符串输入到当前名称的映射。
// 日志等历史数据可能保存旧 code 或 alias，因此展示层不能只解析当前启用 code。
func GetGroupDisplayNameMap() (map[string]string, error) {
	var groups []Group
	if err := DB.Model(&Group{}).
		Select("id", "code", "name").
		Order("id ASC").
		Find(&groups).Error; err != nil {
		return nil, err
	}

	result := make(map[string]string, len(groups)*3)
	nameByID := make(map[int]string, len(groups))
	for _, group := range groups {
		if isVirtualAutoCode(group.Code) {
			continue
		}
		name := strings.TrimSpace(group.Name)
		if name == "" {
			name = strings.TrimSpace(group.Code)
		}
		if name == "" {
			continue
		}
		nameByID[group.Id] = name
		if code := strings.TrimSpace(group.Code); code != "" {
			if _, exists := result[code]; !exists {
				result[code] = name
			}
		}
	}

	// 展示输入沿用正式解析语义：当前 code 优先，其次是历史 alias。
	// 同一优先级按稳定 ID 顺序保留第一项，避免脏数据导致结果随查询顺序变化。
	if DB.Migrator().HasTable(&GroupAlias{}) {
		var aliases []GroupAlias
		if err := DB.Model(&GroupAlias{}).
			Select("id", "alias", "group_id").
			Order("id ASC").
			Find(&aliases).Error; err != nil {
			return nil, err
		}
		for _, alias := range aliases {
			value := strings.TrimSpace(alias.Alias)
			name := nameByID[alias.GroupId]
			if value == "" || name == "" {
				continue
			}
			if _, exists := result[value]; !exists {
				result[value] = name
			}
		}
	}

	// Name 只用于展示兼容，不能覆盖可参与业务解析的 code 或 alias。
	for _, group := range groups {
		if isVirtualAutoCode(group.Code) {
			continue
		}
		name := nameByID[group.Id]
		if name == "" {
			continue
		}
		if _, exists := result[name]; !exists {
			result[name] = name
		}
	}
	return result, nil
}

func GetGroupById(id int) (*Group, error) {
	var group Group
	if err := DB.First(&group, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if isVirtualAutoCode(group.Code) {
		return nil, gorm.ErrRecordNotFound
	}
	var member AutoGroupMember
	if err := DB.First(&member, "group_id = ?", id).Error; err == nil {
		group.AutoEnabled = true
		group.AutoOrder = member.Position
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &group, nil
}

// GetGroupByCodeOrAlias 用于兼容旧的字符串入口。
func GetGroupByCodeOrAlias(code string) (*Group, error) {
	return GetGroupByCodeOrAliasWithDB(DB, code)
}

// GetGroupByCodeOrAliasWithDB 在事务中解析旧字符串入口。
func GetGroupByCodeOrAliasWithDB(tx *gorm.DB, code string) (*Group, error) {
	code = strings.TrimSpace(code)
	if code == "" || isVirtualAutoCode(code) {
		return nil, gorm.ErrRecordNotFound
	}
	var group Group
	err := tx.Where("code = ?", code).First(&group).Error
	if err == nil {
		return &group, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if !tx.Migrator().HasTable(&GroupAlias{}) {
		return nil, gorm.ErrRecordNotFound
	}
	var alias GroupAlias
	if err = tx.Where("alias = ?", code).First(&alias).Error; err != nil {
		return nil, err
	}
	if err = tx.First(&group, "id = ?", alias.GroupId).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func groupLegacyIdentifiers(tx *gorm.DB, group *Group) ([]string, map[string]struct{}, error) {
	if group == nil {
		return nil, nil, errors.New("group is nil")
	}
	identifiers := []string{group.Code}
	identifierSet := map[string]struct{}{group.Code: {}}
	if tx.Migrator().HasTable(&GroupAlias{}) {
		var aliases []string
		if err := tx.Model(&GroupAlias{}).
			Where("group_id = ?", group.Id).
			Order("id ASC").
			Pluck("alias", &aliases).Error; err != nil {
			return nil, nil, err
		}
		for _, alias := range aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			// 当前 code 是正式身份。历史 alias 若与其他组的当前 code 冲突，
			// 该字符串必须归当前 code 所属组，不能再作为本组的兼容引用。
			resolvedGroup, resolveErr := GetGroupByCodeOrAliasWithDB(tx, alias)
			if resolveErr != nil {
				return nil, nil, resolveErr
			}
			if resolvedGroup.Id != group.Id {
				continue
			}
			if _, exists := identifierSet[alias]; exists {
				continue
			}
			identifiers = append(identifiers, alias)
			identifierSet[alias] = struct{}{}
		}
	}
	return identifiers, identifierSet, nil
}

// ResolveGroupLogIdentifiers 将日志筛选输入解析为可匹配的历史分组标识。
// 解析顺序与正式字符串入口一致：当前 code 优先，其次是 alias，最后才按显示名称查找。
// 返回值包含当前 code 和未被其他当前 code 覆盖的历史 alias；停用分组同样保留。
func ResolveGroupLogIdentifiers(identifier string) ([]string, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || isVirtualAutoCode(identifier) {
		return nil, gorm.ErrRecordNotFound
	}

	group, err := GetGroupByCodeOrAliasWithDB(DB, identifier)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}

		var namedGroup Group
		if err = DB.Where("name = ?", identifier).First(&namedGroup).Error; err != nil {
			return nil, err
		}
		group = &namedGroup
	}
	if group == nil || isVirtualAutoCode(group.Code) {
		return nil, gorm.ErrRecordNotFound
	}

	identifiers, _, err := groupLegacyIdentifiers(DB, group)
	if err != nil {
		return nil, err
	}
	return identifiers, nil
}

func legacyGroupSubstringPattern(group string) string {
	group = strings.NewReplacer(
		"!", "!!",
		"%", "!%",
		"_", "!_",
	).Replace(group)
	return "%" + group + "%"
}

func containsLegacyGroupIdentifier(value string, identifiers map[string]struct{}) bool {
	for _, code := range splitLegacyGroupCodes(value) {
		if _, exists := identifiers[code]; exists {
			return true
		}
	}
	return false
}

func ResolveGroupIDByCode(code string) (int, error) {
	group, err := GetGroupByCodeOrAlias(code)
	if err != nil {
		return 0, err
	}
	return group.Id, nil
}

func ResolveGroupIDByCodeWithDB(tx *gorm.DB, code string) (int, error) {
	group, err := GetGroupByCodeOrAliasWithDB(tx, code)
	if err != nil {
		return 0, err
	}
	return group.Id, nil
}

func GetGroupsByIds(ids []int) (map[int]*Group, error) {
	result := make(map[int]*Group, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var groups []*Group
	if err := DB.Where("id IN ?", ids).Find(&groups).Error; err != nil {
		return nil, err
	}
	for _, group := range groups {
		result[group.Id] = group
	}
	return result, nil
}

// ResolveGroupIdsByCodes 将旧 API 的字符串分组解析为 ID，并保留输入顺序。
func ResolveGroupIdsByCodes(codes []string) ([]int, error) {
	ids := make([]int, 0, len(codes))
	seen := make(map[int]struct{}, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		group, err := GetGroupByCodeOrAlias(code)
		if err != nil {
			return nil, fmt.Errorf("分组 %q 不存在", code)
		}
		if _, ok := seen[group.Id]; ok {
			continue
		}
		seen[group.Id] = struct{}{}
		ids = append(ids, group.Id)
	}
	return ids, nil
}

func ResolveGroupCodesByIds(ids []int) ([]string, error) {
	groups, err := GetGroupsByIds(ids)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		group, ok := groups[id]
		if !ok {
			return nil, fmt.Errorf("分组 ID %d 不存在", id)
		}
		seen[id] = struct{}{}
		codes = append(codes, group.Code)
	}
	return codes, nil
}

func collectGroupCode(set map[string]struct{}, value string, allowAuto bool) {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !allowAuto && strings.EqualFold(item, "auto") {
			continue
		}
		if _, err := NormalizeGroupCode(item); err != nil {
			// 迁移阶段不让一个历史脏值阻塞整个服务启动；保留原值供管理员修复。
			continue
		}
		set[item] = struct{}{}
	}
}

func readOptionValue(options map[string]string, key string) string {
	return strings.TrimSpace(options[key])
}

func legacyAutoTokenExists(tx *gorm.DB) (bool, error) {
	if !hasModelColumns(tx, &Token{}, "Group") {
		return false, nil
	}
	query := tx.Model(&Token{}).Select("id")
	if hasModelColumns(tx, &Token{}, "GroupMode") {
		query = query.Where(commonGroupCol+" = ? OR group_mode = ?", TokenGroupModeAuto, TokenGroupModeAuto)
	} else {
		query = query.Where(commonGroupCol+" = ?", TokenGroupModeAuto)
	}
	var token Token
	if err := query.Take(&token).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func resolveAutoGroupConfigFromOptions(tx *gorm.DB, options map[string]string) (setting.AutoGroupConfig, error) {
	if raw := readOptionValue(options, "AutoGroupConfig"); raw != "" {
		var config setting.AutoGroupConfig
		if err := common.UnmarshalJsonStr(raw, &config); err != nil {
			return setting.AutoGroupConfig{}, fmt.Errorf("解析自动分组配置失败: %w", err)
		}
		if strings.EqualFold(readOptionValue(options, "DefaultUseAutoGroup"), "true") {
			config.UserSelectable = true
		}
		return setting.NormalizeAutoGroupConfig(config), nil
	}

	config := setting.NormalizeAutoGroupConfig(setting.AutoGroupConfig{})
	if hasModelColumns(tx, &Group{}, "Code") {
		var legacyAuto Group
		err := tx.Where("LOWER(code) = ?", TokenGroupModeAuto).First(&legacyAuto).Error
		if err == nil {
			config.UserSelectable = legacyAuto.UserSelectable
			if description := strings.TrimSpace(legacyAuto.Description); description != "" {
				config.Description = description
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return setting.AutoGroupConfig{}, err
		}
	}
	if raw := readOptionValue(options, "UserUsableGroups"); raw != "" {
		var groups map[string]string
		if err := common.UnmarshalJsonStr(raw, &groups); err == nil {
			if description, ok := groups[TokenGroupModeAuto]; ok {
				config.UserSelectable = true
				config.Description = strings.TrimSpace(description)
			}
		}
	}
	if strings.EqualFold(readOptionValue(options, "DefaultUseAutoGroup"), "true") {
		config.UserSelectable = true
	}
	if !config.UserSelectable {
		exists, err := legacyAutoTokenExists(tx)
		if err != nil {
			return setting.AutoGroupConfig{}, err
		}
		config.UserSelectable = exists
	}
	return setting.NormalizeAutoGroupConfig(config), nil
}

func loadAutoGroupConfigFromDB(tx *gorm.DB) (setting.AutoGroupConfig, error) {
	keys := []string{"AutoGroupConfig", "UserUsableGroups", "DefaultUseAutoGroup"}
	var rows []Option
	if err := tx.Where(commonKeyCol+" IN ?", keys).Find(&rows).Error; err != nil {
		return setting.AutoGroupConfig{}, err
	}
	options := make(map[string]string, len(rows))
	for _, row := range rows {
		options[row.Key] = row.Value
	}
	return resolveAutoGroupConfigFromOptions(tx, options)
}

func hasModelColumns(tx *gorm.DB, model interface{}, fields ...string) bool {
	if tx == nil || !tx.Migrator().HasTable(model) {
		return false
	}
	for _, field := range fields {
		if !tx.Migrator().HasColumn(model, field) {
			return false
		}
	}
	return true
}

func pluckLegacyGroupValues(tx *gorm.DB, model interface{}, fieldName, columnName string) ([]string, error) {
	if !hasModelColumns(tx, model, fieldName) {
		return nil, nil
	}
	var nullableValues []sql.NullString
	if err := tx.Unscoped().Model(model).Pluck(columnName, &nullableValues).Error; err != nil {
		return nil, err
	}
	values := make([]string, 0, len(nullableValues))
	for _, value := range nullableValues {
		if value.Valid {
			values = append(values, value.String)
		}
	}
	return values, nil
}

func collectLegacyGroupValues(codes map[string]struct{}, values []string) {
	for _, value := range values {
		collectGroupCode(codes, value, false)
	}
}

func legacyGroupNameCandidate(code string, attempt int) string {
	if attempt == 0 {
		return code
	}
	if attempt == 1 {
		return code + " (legacy)"
	}
	return fmt.Sprintf("%s (legacy %d)", code, attempt)
}

func findAvailableLegacyGroupName(tx *gorm.DB, code string, excludeID int) (string, error) {
	const maxNameAttempts = 10000
	for attempt := 0; attempt < maxNameAttempts; attempt++ {
		candidate := legacyGroupNameCandidate(code, attempt)
		var occupied Group
		query := lockForUpdate(tx).Select("id").Where("name = ?", candidate)
		if excludeID > 0 {
			query = query.Where("id <> ?", excludeID)
		}
		err := query.First(&occupied).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("为历史分组 %q 分配显示名称失败：候选名称冲突过多", code)
}

func ensureMySQLGroupIdentityCaseSensitivity(tx *gorm.DB) error {
	if !common.UsingMySQL {
		return nil
	}
	columns := []struct {
		table  string
		column string
	}{
		{table: "groups", column: "code"},
		{table: "group_aliases", column: "alias"},
	}
	for _, column := range columns {
		var collation string
		result := tx.Raw(`SELECT COLLATION_NAME FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			column.table, column.column).Scan(&collation)
		if result.Error != nil {
			return fmt.Errorf("读取 %s.%s 排序规则失败: %w", column.table, column.column, result.Error)
		}
		if result.RowsAffected != 1 || strings.TrimSpace(collation) == "" {
			return fmt.Errorf("读取 %s.%s 排序规则失败: 未找到目标列", column.table, column.column)
		}
		if strings.EqualFold(collation, "utf8mb4_bin") {
			continue
		}
		statement := fmt.Sprintf(
			"ALTER TABLE `%s` MODIFY COLUMN `%s` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL",
			column.table,
			column.column,
		)
		if err := tx.Exec(statement).Error; err != nil {
			return fmt.Errorf("迁移 %s.%s 为大小写敏感排序规则失败: %w", column.table, column.column, err)
		}
	}
	return nil
}

func createLegacyGroup(tx *gorm.DB, template Group) (*Group, error) {
	if template.Ratio == 0 {
		template.Ratio = 1
	}
	const maxNameAttempts = 10000
	for attempt := 0; attempt < maxNameAttempts; attempt++ {
		candidate := template
		candidate.Id = 0
		candidate.Name = legacyGroupNameCandidate(template.Code, attempt)
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
			return nil, err
		}

		// 同 code 的并发创建会落到这里；显示名冲突时则继续尝试下一个候选名。
		var stored Group
		// MySQL 默认 REPEATABLE READ 下需要当前读，才能看到冲突事务刚提交的记录。
		err := lockForUpdate(tx).Where("code = ?", template.Code).First(&stored).Error
		if err == nil {
			return &stored, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("为历史分组 %q 分配显示名称失败：候选名称冲突过多", template.Code)
}

// migrateGroupIdentity 将旧配置和旧表中的名称收敛到 groups。该过程只新增/更新缺失
// 数据，重复执行不会改变已有 ID 或显示名称。
func migrateGroupIdentity() error {
	if err := ensureMySQLGroupIdentityCaseSensitivity(DB); err != nil {
		return err
	}
	options := make(map[string]string)
	var rows []Option
	if err := DB.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		options[row.Key] = row.Value
	}
	autoGroupConfig, err := resolveAutoGroupConfigFromOptions(DB, options)
	if err != nil {
		return err
	}
	autoGroupConfigJSON, err := common.Marshal(autoGroupConfig)
	if err != nil {
		return err
	}
	shouldMigrateAutoGroupConfig := readOptionValue(options, "AutoGroupConfig") == ""

	codes := map[string]struct{}{"default": {}}
	ratioValues := make(map[string]float64)
	descriptions := make(map[string]string)
	selectable := make(map[string]bool)

	for _, key := range []string{"GroupRatio", "group_ratio_setting.group_ratio"} {
		var values map[string]float64
		if raw := readOptionValue(options, key); raw != "" {
			if err := common.UnmarshalJsonStr(raw, &values); err == nil {
				for code, ratio := range values {
					code = strings.TrimSpace(code)
					if code == "" || strings.EqualFold(code, "auto") || strings.EqualFold(code, "all") || strings.EqualFold(code, "null") {
						continue
					}
					codes[code] = struct{}{}
					ratioValues[code] = ratio
				}
			}
		}
	}
	if raw := readOptionValue(options, "UserUsableGroups"); raw != "" {
		var values map[string]string
		if err := common.UnmarshalJsonStr(raw, &values); err == nil {
			for code, description := range values {
				code = strings.TrimSpace(code)
				if code == "" || strings.EqualFold(code, "auto") || strings.EqualFold(code, "all") || strings.EqualFold(code, "null") {
					continue
				}
				codes[code] = struct{}{}
				descriptions[code] = description
				selectable[code] = true
			}
		}
	}
	if raw := readOptionValue(options, "AutoGroups"); raw != "" {
		var values []string
		if err := common.UnmarshalJsonStr(raw, &values); err == nil {
			for _, code := range values {
				collectGroupCode(codes, code, false)
			}
		}
	}
	for _, key := range []string{groupGroupRatioOptionKey, layeredGroupGroupRatioOptionKey} {
		if raw := readOptionValue(options, key); raw != "" {
			var values map[string]map[string]float64
			if err := common.UnmarshalJsonStr(raw, &values); err == nil {
				for owner, targets := range values {
					collectGroupCode(codes, owner, false)
					for target := range targets {
						collectGroupCode(codes, target, false)
					}
				}
			}
		}
	}
	if raw := readOptionValue(options, "TopupGroupRatio"); raw != "" {
		var values map[string]float64
		if err := common.UnmarshalJsonStr(raw, &values); err == nil {
			for code := range values {
				collectGroupCode(codes, code, false)
			}
		}
	}
	if raw := readOptionValue(options, "group_ratio_setting.group_special_usable_group"); raw != "" {
		var values map[string]map[string]string
		if err := common.UnmarshalJsonStr(raw, &values); err == nil {
			for owner, rules := range values {
				collectGroupCode(codes, owner, false)
				for target := range rules {
					target = strings.TrimPrefix(target, "+:")
					target = strings.TrimPrefix(target, "-:")
					collectGroupCode(codes, target, false)
				}
			}
		}
	}
	if raw := readOptionValue(options, "ModelRequestRateLimitGroup"); raw != "" {
		var values map[string][]int
		if err := common.UnmarshalJsonStr(raw, &values); err == nil {
			for code := range values {
				collectGroupCode(codes, code, false)
			}
		}
	}
	if raw := readOptionValue(options, "ModelRequestRateLimitUserGroup"); raw != "" {
		var values map[string]struct {
			Global *[2]int           `json:"global"`
			Groups map[string][2]int `json:"groups"`
		}
		if err := common.UnmarshalJsonStr(raw, &values); err == nil {
			for owner, config := range values {
				collectGroupCode(codes, owner, false)
				for target := range config.Groups {
					collectGroupCode(codes, target, false)
				}
			}
		}
	}

	// 现有业务表仍以名称保存；迁移时把它们纳入实体集合。
	legacyColumns := []struct {
		model      interface{}
		fieldName  string
		columnName string
	}{
		{model: &Channel{}, fieldName: "Group", columnName: "group"},
		{model: &Token{}, fieldName: "Group", columnName: "group"},
		{model: &User{}, fieldName: "Group", columnName: "group"},
		{model: &Ability{}, fieldName: "Group", columnName: "group"},
		{model: &SubscriptionPlan{}, fieldName: "UpgradeGroup", columnName: "upgrade_group"},
		{model: &UserSubscription{}, fieldName: "UpgradeGroup", columnName: "upgrade_group"},
		{model: &UserSubscription{}, fieldName: "PrevUserGroup", columnName: "prev_user_group"},
	}
	for _, legacyColumn := range legacyColumns {
		values, err := pluckLegacyGroupValues(
			DB,
			legacyColumn.model,
			legacyColumn.fieldName,
			legacyColumn.columnName,
		)
		if err != nil {
			return err
		}
		collectLegacyGroupValues(codes, values)
	}

	orderedCodes := make([]string, 0, len(codes))
	for code := range codes {
		if normalized, err := NormalizeGroupCode(code); err == nil {
			orderedCodes = append(orderedCodes, normalized)
		}
	}
	sort.Strings(orderedCodes)

	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		if shouldMigrateAutoGroupConfig {
			option := Option{Key: "AutoGroupConfig", Value: string(autoGroupConfigJSON)}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
				return fmt.Errorf("迁移自动分组配置失败: %w", err)
			}
		}
		for _, code := range orderedCodes {
			var group Group
			err := tx.Where("code = ?", code).First(&group).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				_, createErr := createLegacyGroup(tx, Group{
					Code:           code,
					Description:    descriptions[code],
					Ratio:          ratioValues[code],
					UserSelectable: selectable[code],
					Status:         GroupStatusActive,
					CreatedTime:    now,
					UpdatedTime:    now,
				})
				if createErr != nil {
					return createErr
				}
				continue
			}
			if err != nil {
				return err
			}
			updates := map[string]interface{}{}
			if group.Name == "" {
				name, nameErr := findAvailableLegacyGroupName(tx, code, group.Id)
				if nameErr != nil {
					return nameErr
				}
				updates["name"] = name
			}
			if group.Ratio == 0 {
				if ratio, ok := ratioValues[code]; ok && ratio > 0 {
					updates["ratio"] = ratio
				}
			}
			if group.Description == "" && descriptions[code] != "" {
				updates["description"] = descriptions[code]
			}
			if !group.UserSelectable && selectable[code] {
				updates["user_selectable"] = true
			}
			if len(updates) > 0 {
				updates["updated_time"] = now
				if err := tx.Model(&Group{}).Where("id = ?", group.Id).Updates(updates).Error; err != nil {
					return err
				}
			}
		}

		// 为现有用户和能力记录补齐稳定 ID；旧字符串字段保留作为兼容快照。
		if hasModelColumns(tx, &User{}, "Group", "GroupId") {
			var users []User
			if err := tx.Find(&users).Error; err != nil {
				return err
			}
			for _, user := range users {
				if _, err := NormalizeGroupCode(user.Group); err != nil {
					continue
				}
				groupID, resolveErr := ResolveGroupIDByCodeWithDB(tx, user.Group)
				if errors.Is(resolveErr, gorm.ErrRecordNotFound) {
					continue
				}
				if resolveErr != nil {
					return fmt.Errorf("回填用户 %d 分组 ID 失败: %w", user.Id, resolveErr)
				}
				if groupID > 0 && user.GroupId != groupID {
					if err := tx.Model(&User{}).Where("id = ?", user.Id).Update("group_id", groupID).Error; err != nil {
						return err
					}
				}
			}
		}
		if hasModelColumns(tx, &Ability{}, "Group", "GroupId") {
			var abilities []Ability
			if err := tx.Find(&abilities).Error; err != nil {
				return err
			}
			for _, ability := range abilities {
				if _, err := NormalizeGroupCode(ability.Group); err != nil {
					continue
				}
				groupID, resolveErr := ResolveGroupIDByCodeWithDB(tx, ability.Group)
				if errors.Is(resolveErr, gorm.ErrRecordNotFound) {
					continue
				}
				if resolveErr != nil {
					return fmt.Errorf("回填渠道 %d 模型 %q 的能力分组 ID 失败: %w", ability.ChannelId, ability.Model, resolveErr)
				}
				if groupID > 0 && ability.GroupId != groupID {
					if err := tx.Model(&Ability{}).
						Where("channel_id = ? AND model = ? AND "+commonGroupCol+" = ?", ability.ChannelId, ability.Model, ability.Group).
						Update("group_id", groupID).Error; err != nil {
						return err
					}
				}
			}
		}

		var autoCodes []string
		if raw := readOptionValue(options, "AutoGroups"); raw != "" {
			_ = common.UnmarshalJsonStr(raw, &autoCodes)
		}
		if len(autoCodes) > 0 {
			if err := tx.Where("1 = 1").Delete(&AutoGroupMember{}).Error; err != nil {
				return err
			}
			position := 0
			seen := make(map[int]struct{})
			for _, code := range autoCodes {
				code = strings.TrimSpace(code)
				if code == "" || strings.EqualFold(code, "auto") {
					continue
				}
				if _, err := NormalizeGroupCode(code); err != nil {
					continue
				}
				var group Group
				if err := tx.Where("code = ?", code).First(&group).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						continue
					}
					return fmt.Errorf("回填自动分组 %q 失败: %w", code, err)
				}
				if _, ok := seen[group.Id]; ok {
					continue
				}
				seen[group.Id] = struct{}{}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&AutoGroupMember{GroupId: group.Id, Position: position}).Error; err != nil {
					return err
				}
				position++
			}
		}
		return nil
	})
}

// MigrateGroupIdentity 暴露给迁移测试和启动流程。
func MigrateGroupIdentity() error { return migrateGroupIdentity() }

func buildGroupOptionProjection(tx *gorm.DB) (map[string]string, error) {
	groups, err := GetAllGroupsFromDB(tx)
	if err != nil {
		return nil, err
	}
	ratio := make(map[string]float64, len(groups))
	usable := make(map[string]string)
	for _, group := range groups {
		if group.Status != GroupStatusActive {
			continue
		}
		ratio[group.Code] = group.Ratio
		if group.UserSelectable {
			usable[group.Code] = group.Description
		}
	}
	var members []AutoGroupMember
	if err := tx.Order("position ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	byPosition := make(map[int]int)
	for _, member := range members {
		byPosition[member.Position] = member.GroupId
	}
	auto := make([]string, 0, len(members))
	for position := 0; position < len(members); position++ {
		groupID, ok := byPosition[position]
		if !ok {
			continue
		}
		for _, group := range groups {
			if group.Id == groupID {
				auto = append(auto, group.Code)
				break
			}
		}
	}
	autoGroupConfig, err := loadAutoGroupConfigFromDB(tx)
	if err != nil {
		return nil, err
	}
	if autoGroupConfig.UserSelectable {
		usable[TokenGroupModeAuto] = autoGroupConfig.Description
	}
	ratioJSON, err := common.Marshal(ratio)
	if err != nil {
		return nil, err
	}
	usableJSON, err := common.Marshal(usable)
	if err != nil {
		return nil, err
	}
	autoJSON, err := common.Marshal(auto)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"GroupRatio":                      string(ratioJSON),
		"group_ratio_setting.group_ratio": string(ratioJSON),
		"UserUsableGroups":                string(usableJSON),
		"AutoGroups":                      string(autoJSON),
	}, nil
}

func GetAllGroupsFromDB(tx *gorm.DB) ([]*Group, error) {
	query := tx.Model(&Group{}).
		Where("LOWER(code) <> ?", TokenGroupModeAuto).
		Order("id ASC")
	var groups []*Group
	if err := query.Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func upsertOption(tx *gorm.DB, key, value string) error {
	option := Option{Key: key}
	if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
		return err
	}
	return tx.Model(&Option{}).Where(commonKeyCol+" = ?", key).Update("value", value).Error
}

var groupReferenceOptionKeys = []string{
	groupGroupRatioOptionKey,
	layeredGroupGroupRatioOptionKey,
	"TopupGroupRatio",
	"group_ratio_setting.group_special_usable_group",
	"ModelRequestRateLimitGroup",
	"ModelRequestRateLimitUserGroup",
}

var groupProjectionOptionKeys = []string{
	"GroupRatio",
	"group_ratio_setting.group_ratio",
	"UserUsableGroups",
	"AutoGroups",
}

var groupConfigEditableOptionKeys = []string{
	"AutoGroupConfig",
	"DefaultUseAutoGroup",
	groupGroupRatioOptionKey,
	layeredGroupGroupRatioOptionKey,
	"TopupGroupRatio",
	"group_ratio_setting.group_special_usable_group",
}

var groupConfigJSONOptionKeys = map[string]struct{}{
	"AutoGroupConfig":                                {},
	groupGroupRatioOptionKey:                         {},
	layeredGroupGroupRatioOptionKey:                  {},
	"TopupGroupRatio":                                {},
	"group_ratio_setting.group_special_usable_group": {},
}

func normalizeGroupConfigOptionUpdates(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		if _, isJSONMap := groupConfigJSONOptionKeys[key]; isJSONMap && strings.TrimSpace(value) == "" {
			value = "{}"
		}
		normalized[key] = value
	}
	return normalized
}

func rewrittenTemporaryGroupCode(code string, replacements map[string]string) string {
	trimmed := strings.TrimSpace(code)
	if replacement, exists := replacements[trimmed]; exists {
		return replacement
	}
	return code
}

func rewriteTemporaryGroupRatioReferences(
	key string,
	value string,
	replacements map[string]string,
) (string, error) {
	values := make(map[string]map[string]float64)
	if err := common.UnmarshalJsonStr(value, &values); err != nil {
		return "", fmt.Errorf("解析分组选项 %s 失败: %w", key, err)
	}
	rewritten := make(map[string]map[string]float64, len(values))
	ownerSources := make(map[string]string, len(values))
	for owner, targets := range values {
		rewrittenOwner := rewrittenTemporaryGroupCode(owner, replacements)
		if previous, exists := ownerSources[rewrittenOwner]; exists && previous != owner {
			return "", fmt.Errorf("分组选项 %s 的临时分组 %q 与 %q 改写后发生冲突", key, previous, owner)
		}
		ownerSources[rewrittenOwner] = owner
		rewrittenTargets := make(map[string]float64, len(targets))
		targetSources := make(map[string]string, len(targets))
		for target, ratio := range targets {
			rewrittenTarget := rewrittenTemporaryGroupCode(target, replacements)
			if previous, exists := targetSources[rewrittenTarget]; exists && previous != target {
				return "", fmt.Errorf("分组选项 %s 的临时分组 %q 与 %q 改写后发生冲突", key, previous, target)
			}
			targetSources[rewrittenTarget] = target
			rewrittenTargets[rewrittenTarget] = ratio
		}
		rewritten[rewrittenOwner] = rewrittenTargets
	}
	return marshalPrunedGroupOption(key, rewritten)
}

func rewriteTemporaryTopupGroupRatioReferences(
	key string,
	value string,
	replacements map[string]string,
) (string, error) {
	values := make(map[string]float64)
	if err := common.UnmarshalJsonStr(value, &values); err != nil {
		return "", fmt.Errorf("解析分组选项 %s 失败: %w", key, err)
	}
	rewritten := make(map[string]float64, len(values))
	sources := make(map[string]string, len(values))
	for code, ratio := range values {
		rewrittenCode := rewrittenTemporaryGroupCode(code, replacements)
		if previous, exists := sources[rewrittenCode]; exists && previous != code {
			return "", fmt.Errorf("分组选项 %s 的临时分组 %q 与 %q 改写后发生冲突", key, previous, code)
		}
		sources[rewrittenCode] = code
		rewritten[rewrittenCode] = ratio
	}
	return marshalPrunedGroupOption(key, rewritten)
}

func rewriteTemporarySpecialGroupTarget(target string, replacements map[string]string) string {
	trimmed := strings.TrimSpace(target)
	prefix := ""
	identifier := trimmed
	if strings.HasPrefix(identifier, "+:") || strings.HasPrefix(identifier, "-:") {
		prefix = identifier[:2]
		identifier = strings.TrimSpace(identifier[2:])
	}
	replacement, exists := replacements[identifier]
	if !exists {
		return target
	}
	return prefix + replacement
}

func rewriteTemporarySpecialUsableGroupReferences(
	key string,
	value string,
	replacements map[string]string,
) (string, error) {
	values := make(map[string]map[string]string)
	if err := common.UnmarshalJsonStr(value, &values); err != nil {
		return "", fmt.Errorf("解析分组选项 %s 失败: %w", key, err)
	}
	rewritten := make(map[string]map[string]string, len(values))
	ownerSources := make(map[string]string, len(values))
	for owner, rules := range values {
		rewrittenOwner := rewrittenTemporaryGroupCode(owner, replacements)
		if previous, exists := ownerSources[rewrittenOwner]; exists && previous != owner {
			return "", fmt.Errorf("分组选项 %s 的临时分组 %q 与 %q 改写后发生冲突", key, previous, owner)
		}
		ownerSources[rewrittenOwner] = owner
		rewrittenRules := make(map[string]string, len(rules))
		targetSources := make(map[string]string, len(rules))
		for target, rule := range rules {
			rewrittenTarget := rewriteTemporarySpecialGroupTarget(target, replacements)
			if previous, exists := targetSources[rewrittenTarget]; exists && previous != target {
				return "", fmt.Errorf("分组选项 %s 的临时分组规则 %q 与 %q 改写后发生冲突", key, previous, target)
			}
			targetSources[rewrittenTarget] = target
			rewrittenRules[rewrittenTarget] = rule
		}
		rewritten[rewrittenOwner] = rewrittenRules
	}
	return marshalPrunedGroupOption(key, rewritten)
}

// rewriteTemporaryGroupOptionReferences 把一次保存请求中的临时 code 原子改写为
// 新建分组取得的最终数字 code。临时 code 只用于关联同一请求内的高级配置。
func rewriteTemporaryGroupOptionReferences(
	values map[string]string,
	replacements map[string]string,
) (map[string]string, error) {
	if len(values) == 0 || len(replacements) == 0 {
		return values, nil
	}
	rewritten := make(map[string]string, len(values))
	for key, value := range values {
		var (
			normalized string
			err        error
		)
		switch key {
		case groupGroupRatioOptionKey, layeredGroupGroupRatioOptionKey:
			normalized, err = rewriteTemporaryGroupRatioReferences(key, value, replacements)
		case "TopupGroupRatio":
			normalized, err = rewriteTemporaryTopupGroupRatioReferences(key, value, replacements)
		case "group_ratio_setting.group_special_usable_group":
			normalized, err = rewriteTemporarySpecialUsableGroupReferences(key, value, replacements)
		default:
			normalized = value
		}
		if err != nil {
			return nil, err
		}
		rewritten[key] = normalized
	}
	return rewritten, nil
}

func validateGroupConfigOptionUpdates(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(groupConfigEditableOptionKeys))
	for _, key := range groupConfigEditableOptionKeys {
		allowed[key] = struct{}{}
	}
	projectionKeys := make(map[string]struct{}, len(groupProjectionOptionKeys))
	for _, key := range groupProjectionOptionKeys {
		projectionKeys[key] = struct{}{}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	keys = sortedUniqueOptionKeys(keys)
	for _, key := range keys {
		if _, projected := projectionKeys[key]; projected {
			return fmt.Errorf("分组选项 %s 由分组配置自动生成，不能直接提交", key)
		}
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("分组配置不允许修改选项 %s", key)
		}
		if err := validateOptionValue(key, values[key]); err != nil {
			return fmt.Errorf("分组选项 %s 校验失败: %w", key, err)
		}
	}
	return nil
}

func groupConfigManagedOptionKeys() []string {
	keys := make([]string, 0, len(groupReferenceOptionKeys)+len(groupProjectionOptionKeys)+len(groupConfigEditableOptionKeys))
	keys = append(keys, groupReferenceOptionKeys...)
	keys = append(keys, groupProjectionOptionKeys...)
	keys = append(keys, groupConfigEditableOptionKeys...)
	return keys
}

func groupReferenceOptionGroupIDs(tx *gorm.DB, key string, value string) ([]int, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	identifiers := make(map[string]struct{})
	addIdentifier := func(identifier string) {
		identifier = strings.TrimSpace(identifier)
		if identifier != "" {
			identifiers[identifier] = struct{}{}
		}
	}
	switch key {
	case "GroupRatio", "group_ratio_setting.group_ratio":
		values := make(map[string]float64)
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return nil, fmt.Errorf("解析分组选项 %s 失败: %w", key, err)
		}
		for identifier := range values {
			addIdentifier(identifier)
		}
	case "UserUsableGroups":
		values := make(map[string]string)
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return nil, fmt.Errorf("解析分组选项 %s 失败: %w", key, err)
		}
		for identifier := range values {
			if strings.EqualFold(strings.TrimSpace(identifier), TokenGroupModeAuto) {
				continue
			}
			addIdentifier(identifier)
		}
	case "AutoGroups":
		var values []string
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return nil, fmt.Errorf("解析分组选项 %s 失败: %w", key, err)
		}
		for _, identifier := range values {
			addIdentifier(identifier)
		}
	case groupGroupRatioOptionKey, layeredGroupGroupRatioOptionKey:
		values := make(map[string]map[string]float64)
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return nil, fmt.Errorf("解析分组选项 %s 失败: %w", key, err)
		}
		for owner, targets := range values {
			addIdentifier(owner)
			for target := range targets {
				addIdentifier(target)
			}
		}
	case "TopupGroupRatio":
		values := make(map[string]float64)
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return nil, fmt.Errorf("解析分组选项 %s 失败: %w", key, err)
		}
		for identifier := range values {
			addIdentifier(identifier)
		}
	case "group_ratio_setting.group_special_usable_group":
		values := make(map[string]map[string]string)
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return nil, fmt.Errorf("解析分组选项 %s 失败: %w", key, err)
		}
		for owner, rules := range values {
			addIdentifier(owner)
			for target := range rules {
				target = strings.TrimSpace(target)
				target = strings.TrimPrefix(target, "+:")
				target = strings.TrimPrefix(target, "-:")
				if strings.EqualFold(strings.TrimSpace(target), TokenGroupModeAuto) {
					continue
				}
				addIdentifier(target)
			}
		}
	case "ModelRequestRateLimitGroup":
		values := make(map[string]setting.RateLimitCounts)
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return nil, fmt.Errorf("解析分组选项 %s 失败: %w", key, err)
		}
		for identifier := range values {
			addIdentifier(identifier)
		}
	case "ModelRequestRateLimitUserGroup":
		values := make(map[string]setting.UserGroupRateLimit)
		if err := common.UnmarshalJsonStr(value, &values); err != nil {
			return nil, fmt.Errorf("解析分组选项 %s 失败: %w", key, err)
		}
		for owner, config := range values {
			addIdentifier(owner)
			for target := range config.Groups {
				addIdentifier(target)
			}
		}
	default:
		return nil, nil
	}

	orderedIdentifiers := make([]string, 0, len(identifiers))
	for identifier := range identifiers {
		orderedIdentifiers = append(orderedIdentifiers, identifier)
	}
	sort.Strings(orderedIdentifiers)
	groupIDs := make([]int, 0, len(orderedIdentifiers))
	for _, identifier := range orderedIdentifiers {
		group, err := getGroupByCodeOrAliasStrict(tx, identifier)
		if err != nil {
			return nil, fmt.Errorf("分组选项 %s 引用了不存在的分组 %q: %w", key, identifier, err)
		}
		groupIDs = append(groupIDs, group.Id)
	}
	return groupIDs, nil
}

func lockGroupReferenceOptionWrite(tx *gorm.DB, key string, value string) error {
	groupIDs, err := groupReferenceOptionGroupIDs(tx, key, value)
	if err != nil {
		return err
	}
	return lockGroupRowsForBindingWrite(tx, groupIDs, "分组选项")
}

func validateStoredGroupReferenceOptions(tx *gorm.DB) error {
	keys := make([]string, 0, len(groupReferenceOptionKeys)+len(groupProjectionOptionKeys))
	keys = append(keys, groupReferenceOptionKeys...)
	keys = append(keys, groupProjectionOptionKeys...)
	keys = sortedUniqueOptionKeys(keys)
	var options []Option
	if err := tx.Where(commonKeyCol+" IN ?", keys).Order(commonKeyCol + " ASC").Find(&options).Error; err != nil {
		return err
	}
	for _, option := range options {
		if _, err := groupReferenceOptionGroupIDs(tx, option.Key, option.Value); err != nil {
			return err
		}
	}
	return nil
}

func matchesDeletedGroupIdentifier(value string, identifiers map[string]struct{}) bool {
	_, exists := identifiers[strings.TrimSpace(value)]
	return exists
}

func marshalPrunedGroupOption(key string, value interface{}) (string, error) {
	data, err := common.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("序列化分组选项 %s 失败: %w", key, err)
	}
	return string(data), nil
}

func pruneDeletedGroupOptionReferences(
	tx *gorm.DB,
	deletedGroups []Group,
) (map[string]string, error) {
	result := make(map[string]string)
	if len(deletedGroups) == 0 {
		return result, nil
	}
	identifiers := make(map[string]struct{})
	for index := range deletedGroups {
		_, groupIdentifiers, err := groupLegacyIdentifiers(tx, &deletedGroups[index])
		if err != nil {
			return nil, fmt.Errorf("加载分组 %s 的兼容标识失败: %w", deletedGroups[index].Name, err)
		}
		for identifier := range groupIdentifiers {
			identifiers[identifier] = struct{}{}
		}
	}

	var options []Option
	if err := tx.Where(commonKeyCol+" IN ?", groupReferenceOptionKeys).Order(commonKeyCol + " ASC").Find(&options).Error; err != nil {
		return nil, err
	}
	for _, option := range options {
		if strings.TrimSpace(option.Value) == "" {
			continue
		}
		changed := false
		var prunedValue string
		switch option.Key {
		case groupGroupRatioOptionKey, layeredGroupGroupRatioOptionKey:
			values := make(map[string]map[string]float64)
			if err := common.UnmarshalJsonStr(option.Value, &values); err != nil {
				return nil, fmt.Errorf("解析分组选项 %s 失败: %w", option.Key, err)
			}
			for owner, targets := range values {
				if matchesDeletedGroupIdentifier(owner, identifiers) {
					delete(values, owner)
					changed = true
					continue
				}
				for target := range targets {
					if matchesDeletedGroupIdentifier(target, identifiers) {
						delete(targets, target)
						changed = true
					}
				}
			}
			if changed {
				var err error
				prunedValue, err = marshalPrunedGroupOption(option.Key, values)
				if err != nil {
					return nil, err
				}
			}
		case "TopupGroupRatio":
			values := make(map[string]float64)
			if err := common.UnmarshalJsonStr(option.Value, &values); err != nil {
				return nil, fmt.Errorf("解析分组选项 %s 失败: %w", option.Key, err)
			}
			for code := range values {
				if matchesDeletedGroupIdentifier(code, identifiers) {
					delete(values, code)
					changed = true
				}
			}
			if changed {
				var err error
				prunedValue, err = marshalPrunedGroupOption(option.Key, values)
				if err != nil {
					return nil, err
				}
			}
		case "group_ratio_setting.group_special_usable_group":
			values := make(map[string]map[string]string)
			if err := common.UnmarshalJsonStr(option.Value, &values); err != nil {
				return nil, fmt.Errorf("解析分组选项 %s 失败: %w", option.Key, err)
			}
			for owner, rules := range values {
				if matchesDeletedGroupIdentifier(owner, identifiers) {
					delete(values, owner)
					changed = true
					continue
				}
				for target := range rules {
					identifier := strings.TrimSpace(target)
					identifier = strings.TrimPrefix(identifier, "+:")
					identifier = strings.TrimPrefix(identifier, "-:")
					if matchesDeletedGroupIdentifier(identifier, identifiers) {
						delete(rules, target)
						changed = true
					}
				}
			}
			if changed {
				var err error
				prunedValue, err = marshalPrunedGroupOption(option.Key, values)
				if err != nil {
					return nil, err
				}
			}
		case "ModelRequestRateLimitGroup":
			values := make(map[string]setting.RateLimitCounts)
			if err := common.UnmarshalJsonStr(option.Value, &values); err != nil {
				return nil, fmt.Errorf("解析分组选项 %s 失败: %w", option.Key, err)
			}
			for code := range values {
				if matchesDeletedGroupIdentifier(code, identifiers) {
					delete(values, code)
					changed = true
				}
			}
			if changed {
				var err error
				prunedValue, err = marshalPrunedGroupOption(option.Key, values)
				if err != nil {
					return nil, err
				}
			}
		case "ModelRequestRateLimitUserGroup":
			values := make(map[string]setting.UserGroupRateLimit)
			if err := common.UnmarshalJsonStr(option.Value, &values); err != nil {
				return nil, fmt.Errorf("解析分组选项 %s 失败: %w", option.Key, err)
			}
			for owner, config := range values {
				if matchesDeletedGroupIdentifier(owner, identifiers) {
					delete(values, owner)
					changed = true
					continue
				}
				for target := range config.Groups {
					if matchesDeletedGroupIdentifier(target, identifiers) {
						delete(config.Groups, target)
						changed = true
					}
				}
				values[owner] = config
			}
			if changed {
				var err error
				prunedValue, err = marshalPrunedGroupOption(option.Key, values)
				if err != nil {
					return nil, err
				}
			}
		}
		if changed {
			result[option.Key] = prunedValue
		}
	}
	return result, nil
}

func countGroupIDReference(tx *gorm.DB, model interface{}, fieldName string, groupID int) (int64, error) {
	if !hasModelColumns(tx, model, fieldName) {
		return 0, nil
	}
	var count int64
	if err := tx.Unscoped().Model(model).Where("group_id = ?", groupID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func countLegacyGroupReferences(
	tx *gorm.DB,
	model interface{},
	fieldName string,
	columnName string,
	identifiers []string,
	identifierSet map[string]struct{},
) (int64, error) {
	if !hasModelColumns(tx, model, fieldName) {
		return 0, nil
	}
	if len(identifiers) == 0 {
		return 0, nil
	}

	column := columnName
	if columnName == "group" {
		column = commonGroupCol
	}
	condition := column + " LIKE ? ESCAPE '!'"
	query := tx.Unscoped().Model(model).Where(condition, legacyGroupSubstringPattern(identifiers[0]))
	for _, identifier := range identifiers[1:] {
		query = query.Or(condition, legacyGroupSubstringPattern(identifier))
	}
	var values []sql.NullString
	if err := query.Pluck(columnName, &values).Error; err != nil {
		return 0, err
	}
	var count int64
	for _, value := range values {
		if value.Valid && containsLegacyGroupIdentifier(value.String, identifierSet) {
			count++
		}
	}
	return count, nil
}

func groupBusinessReferenceCount(tx *gorm.DB, group *Group) (int64, error) {
	if group == nil {
		return 0, nil
	}
	identifiers, identifierSet, err := groupLegacyIdentifiers(tx, group)
	if err != nil {
		return 0, err
	}
	checks := []struct {
		model     interface{}
		fieldName string
	}{
		{model: &ChannelGroupBinding{}, fieldName: "GroupId"},
		{model: &TokenGroupBinding{}, fieldName: "GroupId"},
		{model: &User{}, fieldName: "GroupId"},
		{model: &Ability{}, fieldName: "GroupId"},
	}
	var total int64
	for _, check := range checks {
		count, err := countGroupIDReference(tx, check.model, check.fieldName, group.Id)
		if err != nil {
			return 0, err
		}
		total += count
	}
	legacyChecks := []struct {
		model      interface{}
		fieldName  string
		columnName string
	}{
		{model: &Channel{}, fieldName: "Group", columnName: "group"},
		{model: &Token{}, fieldName: "Group", columnName: "group"},
		{model: &User{}, fieldName: "Group", columnName: "group"},
		{model: &Ability{}, fieldName: "Group", columnName: "group"},
		{model: &SubscriptionPlan{}, fieldName: "UpgradeGroup", columnName: "upgrade_group"},
		{model: &UserSubscription{}, fieldName: "UpgradeGroup", columnName: "upgrade_group"},
		{model: &UserSubscription{}, fieldName: "PrevUserGroup", columnName: "prev_user_group"},
	}
	for _, check := range legacyChecks {
		count, err := countLegacyGroupReferences(
			tx,
			check.model,
			check.fieldName,
			check.columnName,
			identifiers,
			identifierSet,
		)
		if err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func groupConfigWriteLockIDs(configs []GroupConfig, deletedIDs []int) []int {
	ids := make([]int, 0, len(configs)+len(deletedIDs))
	for _, config := range configs {
		if config.Id > 0 {
			ids = append(ids, config.Id)
		}
	}
	ids = append(ids, deletedIDs...)
	sort.Ints(ids)
	unique := ids[:0]
	for _, id := range ids {
		if id <= 0 || (len(unique) > 0 && unique[len(unique)-1] == id) {
			continue
		}
		unique = append(unique, id)
	}
	return unique
}

func lockGroupConfigRowsForWrite(tx *gorm.DB, configs []GroupConfig, deletedIDs []int) (map[int]Group, error) {
	ids := groupConfigWriteLockIDs(configs, deletedIDs)
	lockedByID := make(map[int]Group, len(ids))
	if len(ids) == 0 {
		return lockedByID, nil
	}
	var groups []Group
	if err := lockForUpdate(tx.Model(&Group{})).
		Where("id IN ?", ids).
		Order("id ASC").
		Find(&groups).Error; err != nil {
		return nil, err
	}
	for _, group := range groups {
		lockedByID[group.Id] = group
	}
	return lockedByID, nil
}

// SaveGroupConfigWithOptionsAndAutoConfigResult 原子保存实体分组、虚拟 auto
// 配置和分组页面高级选项。auto 配置缺失时保留数据库中的现有值。
func SaveGroupConfigWithOptionsAndAutoConfigResult(
	configs []GroupConfig,
	deletedIDs []int,
	optionUpdates map[string]string,
	autoConfig *setting.AutoGroupConfig,
) (*GroupConfigSaveResult, error) {
	if autoConfig == nil {
		return SaveGroupConfigWithOptionsAndResult(configs, deletedIDs, optionUpdates)
	}
	updates := make(map[string]string, len(optionUpdates)+1)
	for key, value := range optionUpdates {
		updates[key] = value
	}
	normalized := setting.NormalizeAutoGroupConfig(*autoConfig)
	raw, err := common.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	updates["AutoGroupConfig"] = string(raw)
	return SaveGroupConfigWithOptionsAndResult(configs, deletedIDs, updates)
}

// SaveGroupConfigWithOptionsAndResult 原子保存分组与分组页面高级选项，
// 并返回自动迁移和缓存清理结果。
func SaveGroupConfigWithOptionsAndResult(
	configs []GroupConfig,
	deletedIDs []int,
	optionUpdates map[string]string,
) (*GroupConfigSaveResult, error) {
	if len(configs) == 0 && len(deletedIDs) == 0 && len(optionUpdates) == 0 {
		return nil, errors.New("分组配置不能为空")
	}
	optionUpdates = normalizeGroupConfigOptionUpdates(optionUpdates)
	var normalizeErr error
	optionUpdates, normalizeErr = normalizeGroupGroupRatioOptionUpdates(optionUpdates)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	if err := validateGroupConfigOptionUpdates(optionUpdates); err != nil {
		return nil, err
	}
	optionWriteMutex.Lock()
	defer optionWriteMutex.Unlock()
	result := &GroupConfigSaveResult{}
	projection := map[string]string{}
	migratedTokenPlans := make([]tokenGroupMigrationPlan, 0)
	err := DB.Transaction(func(tx *gorm.DB) error {
		prepared := make([]GroupConfig, len(configs))
		seenCodes := make(map[string]struct{}, len(configs))
		seenNames := make(map[string]struct{}, len(configs))
		seenIDs := make(map[int]struct{}, len(configs))
		temporaryCodes := make(map[int]string)
		temporaryCodeReplacements := make(map[string]string)
		requestedDeletedIDs := make(map[int]struct{}, len(deletedIDs))
		for _, id := range deletedIDs {
			if id > 0 {
				requestedDeletedIDs[id] = struct{}{}
			}
		}
		for index, item := range configs {
			code, err := NormalizeGroupCode(item.Code)
			if err != nil {
				return err
			}
			name, err := normalizeGroupName(item.Name, code)
			if err != nil {
				return err
			}
			if _, ok := seenCodes[code]; ok {
				return fmt.Errorf("分组标识重复: %s", code)
			}
			seenCodes[code] = struct{}{}
			if _, ok := seenNames[name]; ok {
				return fmt.Errorf("分组名称重复: %s", name)
			}
			seenNames[name] = struct{}{}
			if item.Id > 0 {
				if _, ok := seenIDs[item.Id]; ok {
					return fmt.Errorf("分组 ID 重复: %d", item.Id)
				}
				seenIDs[item.Id] = struct{}{}
			}
			if item.Ratio < 0 {
				return fmt.Errorf("分组 %s 的倍率不能小于 0", code)
			}
			if !item.ExclusiveOmitted && item.Exclusive && item.AutoEnabled {
				return fmt.Errorf("独立分组 %s 不能加入自动分组", name)
			}
			item.Code = code
			item.Name = name
			prepared[index] = item
			if item.Id <= 0 {
				temporaryCodes[index] = code
			}
		}

		existingByID := make(map[int]Group, len(seenIDs))
		for index := range prepared {
			item := &prepared[index]
			if item.Id <= 0 {
				// 新客户端提交的 code 只是本次保存请求内的临时引用，但它仍不能
				// 覆盖已有正式 code 或历史 alias。否则高级配置中的同一字符串
				// 会被错误改写到新分组，造成计费和访问控制串组。
				identified, identifyErr := getGroupByCodeOrAliasStrict(tx, item.Code)
				if identifyErr == nil {
					if identified.Name != item.Name {
						return fmt.Errorf(
							"分组临时标识 %s 已属于分组 %s（ID %d）",
							item.Code,
							identified.Name,
							identified.Id,
						)
					}
					if identified.Code != strconv.Itoa(identified.Id) {
						return fmt.Errorf(
							"分组标识 %s 已存在，旧式分组不能作为新建请求重试",
							item.Code,
						)
					}
					if _, explicitlySaved := seenIDs[identified.Id]; explicitlySaved {
						return fmt.Errorf("分组 ID 重复: %d", identified.Id)
					}
					if _, deleted := requestedDeletedIDs[identified.Id]; deleted {
						return fmt.Errorf("分组 ID %d 不能同时保存和删除", identified.Id)
					}
					item.Id = identified.Id
					item.Code = identified.Code
					seenIDs[identified.Id] = struct{}{}
					existingByID[identified.Id] = *identified
					if temporaryCode := temporaryCodes[index]; temporaryCode != identified.Code {
						temporaryCodeReplacements[temporaryCode] = identified.Code
					}
					continue
				}
				if identifyErr != nil && !errors.Is(identifyErr, gorm.ErrRecordNotFound) {
					return identifyErr
				}

				var existing Group
				err := tx.Where("name = ?", item.Name).First(&existing).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				if err != nil {
					return err
				}
				if _, explicitlySaved := seenIDs[existing.Id]; explicitlySaved {
					// 同一请求可能先把旧分组改名，再用释放出的名称创建新分组。
					continue
				}
				if _, deleted := requestedDeletedIDs[existing.Id]; deleted {
					// 删除旧分组并复用其显示名称不是网络重试。
					continue
				}
				// MySQL 常见排序规则不区分大小写，数据库的 name = ? 可能把
				// vip 命中为 VIP。这里只接受 Go 字符串完全一致的网络重试。
				if existing.Name != item.Name {
					return fmt.Errorf("分组名称已存在: %s", existing.Name)
				}
				if existing.Code != strconv.Itoa(existing.Id) {
					return fmt.Errorf(
						"分组名称 %s 已属于旧式标识 %s，不能作为新建请求重试",
						existing.Name,
						existing.Code,
					)
				}
				item.Id = existing.Id
				item.Code = existing.Code
				seenIDs[existing.Id] = struct{}{}
				existingByID[existing.Id] = existing
				if temporaryCode := temporaryCodes[index]; temporaryCode != existing.Code {
					temporaryCodeReplacements[temporaryCode] = existing.Code
				}
				continue
			}
			var existing Group
			if err := tx.First(&existing, "id = ?", item.Id).Error; err != nil {
				return err
			}
			if existing.Code != item.Code {
				return fmt.Errorf("分组 %d 的 code 不允许修改", item.Id)
			}
			existingByID[item.Id] = existing
		}

		seenDeletedIDs := make(map[int]struct{}, len(deletedIDs))
		normalizedDeletedIDs := make([]int, 0, len(deletedIDs))
		for _, id := range deletedIDs {
			if id <= 0 {
				continue
			}
			if _, exists := seenDeletedIDs[id]; exists {
				return fmt.Errorf("待删除分组 ID 重复: %d", id)
			}
			seenDeletedIDs[id] = struct{}{}
			if _, exists := seenIDs[id]; exists {
				return fmt.Errorf("分组 ID %d 不能同时保存和删除", id)
			}
			normalizedDeletedIDs = append(normalizedDeletedIDs, id)
		}
		sort.Ints(normalizedDeletedIDs)

		// 所有现有保存组和待删除组必须在任何选项行之前按统一顺序加锁，
		// 与分组选项写入的 group -> option 锁序保持一致。
		lockedGroups, err := lockGroupConfigRowsForWrite(tx, prepared, normalizedDeletedIDs)
		if err != nil {
			return err
		}
		for index := range prepared {
			item := &prepared[index]
			if item.Id <= 0 {
				if item.Exclusive && item.AutoEnabled {
					return fmt.Errorf("独立分组 %s 不能加入自动分组", item.Name)
				}
				continue
			}
			existing, exists := lockedGroups[item.Id]
			if !exists {
				return fmt.Errorf("分组 ID %d 在保存期间已被删除，请重试", item.Id)
			}
			if existing.Code != item.Code {
				return fmt.Errorf("分组 %d 的 code 不允许修改", item.Id)
			}
			if item.ExclusiveOmitted {
				item.Exclusive = existing.Exclusive
			}
			if item.Exclusive && item.AutoEnabled {
				return fmt.Errorf("独立分组 %s 不能加入自动分组", item.Name)
			}
			existingByID[item.Id] = existing
		}
		deletedGroups := make([]Group, 0, len(normalizedDeletedIDs))
		for _, id := range normalizedDeletedIDs {
			if group, exists := lockedGroups[id]; exists {
				deletedGroups = append(deletedGroups, group)
			}
		}
		for index := range deletedGroups {
			group := &deletedGroups[index]
			if group.Code == "default" {
				return errors.New("default 分组不能删除")
			}
		}
		if len(prepared) == 0 && len(deletedGroups) == 0 && len(optionUpdates) == 0 {
			return nil
		}
		for index := range deletedGroups {
			group := &deletedGroups[index]
			plans, migrationSummary, err := migrateTokenGroupInTx(
				tx,
				group.Id,
				0,
				TokenGroupModeAuto,
			)
			if err != nil {
				return fmt.Errorf("把分组 %s 的令牌迁移到 auto 失败: %w", group.Name, err)
			}
			migratedTokenPlans = append(migratedTokenPlans, plans...)
			result.MigratedTokens += migrationSummary.MigratedTokens
			result.CleanedDeletedTokens += migrationSummary.CleanedDeletedTokens
			referenceCount, err := groupBusinessReferenceCount(tx, group)
			if err != nil {
				return err
			}
			if referenceCount > 0 {
				return fmt.Errorf("分组 %s 仍被非令牌业务数据引用，不能删除", group.Name)
			}
		}

		// 新分组先使用事务内占位名称和 code 取得数据库 ID，再立即把 code
		// 固定为十进制 ID。显示名称要等名称冲突组删除或换名后再写入。
		temporaryNonce := time.Now().UnixNano()
		skippedPlaceholderIDs := make([]int, 0)
		for index := range prepared {
			item := &prepared[index]
			if item.Id > 0 {
				continue
			}
			var group Group
			allocated := false
			for attempt := 0; attempt < 100; attempt++ {
				codeCandidate := fmt.Sprintf("__group_code_pending_%d_%d_%d", temporaryNonce, index, attempt)
				nameCandidate := fmt.Sprintf("__group_name_pending_%d_%d_%d", temporaryNonce, index, attempt)
				var codeCount, nameCount int64
				if err := tx.Model(&Group{}).Where("code = ?", codeCandidate).Count(&codeCount).Error; err != nil {
					return err
				}
				if err := tx.Model(&Group{}).Where("name = ?", nameCandidate).Count(&nameCount).Error; err != nil {
					return err
				}
				if codeCount != 0 || nameCount != 0 {
					continue
				}

				now := time.Now().Unix()
				group = Group{
					Code:           codeCandidate,
					Name:           nameCandidate,
					Description:    item.Description,
					Ratio:          item.Ratio,
					UserSelectable: item.UserSelectable,
					Exclusive:      item.Exclusive,
					Status:         item.Status,
					CreatedTime:    now,
					UpdatedTime:    now,
				}
				if group.Ratio == 0 {
					group.Ratio = 1
				}
				if group.Status == 0 {
					group.Status = GroupStatusActive
				}
				if err := tx.Create(&group).Error; err != nil {
					return fmt.Errorf("创建分组 %s 的事务内占位记录失败: %w", item.Name, err)
				}

				finalCode := strconv.Itoa(group.Id)
				codeHolder, err := getGroupByCodeOrAliasStrict(tx, finalCode)
				if err == nil && codeHolder.Id != group.Id {
					// SQLite 在事务回滚后会复用相同 ROWID。占位行必须暂时保留，
					// 让下一次 Create 取得更大的 ID；成功分配后再统一删除。
					skippedPlaceholderIDs = append(skippedPlaceholderIDs, group.Id)
					continue
				}
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				if err := tx.Model(&Group{}).Where("id = ?", group.Id).Update("code", finalCode).Error; err != nil {
					return fmt.Errorf("把新分组 %s 的标识更新为 ID %s 失败: %w", item.Name, finalCode, err)
				}
				group.Code = finalCode
				allocated = true
				break
			}
			if !allocated {
				return fmt.Errorf("无法为新分组 %s 分配未被占用的数字标识", item.Name)
			}

			if temporaryCode := temporaryCodes[index]; temporaryCode != group.Code {
				temporaryCodeReplacements[temporaryCode] = group.Code
			}
			item.Id = group.Id
			item.Code = group.Code
			item.Ratio = group.Ratio
			item.Status = group.Status
		}
		if len(skippedPlaceholderIDs) > 0 {
			if err := tx.Where("id IN ?", skippedPlaceholderIDs).Delete(&Group{}).Error; err != nil {
				return fmt.Errorf("清理数字标识冲突占位记录失败: %w", err)
			}
		}

		optionUpdates, err = rewriteTemporaryGroupOptionReferences(optionUpdates, temporaryCodeReplacements)
		if err != nil {
			return err
		}
		if err := lockOptionRowsForWrite(tx, groupConfigManagedOptionKeys()); err != nil {
			return err
		}
		normalizedUpdates, err := normalizeAutoGroupOptionUpdatesWithDB(tx, optionUpdates)
		if err != nil {
			return err
		}
		optionUpdates = normalizedUpdates
		optionUpdateKeys := make([]string, 0, len(optionUpdates))
		for key := range optionUpdates {
			optionUpdateKeys = append(optionUpdateKeys, key)
		}
		optionUpdateKeys = sortedUniqueOptionKeys(optionUpdateKeys)
		for _, key := range optionUpdateKeys {
			value := optionUpdates[key]
			if err := upsertOption(tx, key, value); err != nil {
				return err
			}
			projection[key] = value
		}
		prunedOptions, err := pruneDeletedGroupOptionReferences(tx, deletedGroups)
		if err != nil {
			return err
		}
		for key, value := range prunedOptions {
			projection[key] = value
		}
		writeGroupProjection := func() error {
			groupProjection, err := buildGroupOptionProjection(tx)
			if err != nil {
				return err
			}
			for key, value := range groupProjection {
				projection[key] = value
			}
			projectionKeys := make([]string, 0, len(projection))
			for key := range projection {
				projectionKeys = append(projectionKeys, key)
			}
			projectionKeys = sortedUniqueOptionKeys(projectionKeys)
			if err := lockOptionRowsForWrite(tx, projectionKeys); err != nil {
				return err
			}
			for _, key := range projectionKeys {
				if err := upsertOption(tx, key, projection[key]); err != nil {
					return err
				}
			}
			return nil
		}
		if len(prepared) == 0 && len(deletedGroups) == 0 {
			if _, autoConfigChanged := optionUpdates["AutoGroupConfig"]; autoConfigChanged {
				if err := writeGroupProjection(); err != nil {
					return err
				}
			}
			return validateStoredGroupReferenceOptions(tx)
		}

		finalNameByID := make(map[int]string, len(existingByID))
		for _, item := range prepared {
			if item.Id > 0 {
				finalNameByID[item.Id] = item.Name
			}
		}
		for _, item := range prepared {
			var holder Group
			err := tx.Where("name = ?", item.Name).First(&holder).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if holder.Id == item.Id {
				continue
			}
			if _, deleted := seenDeletedIDs[holder.Id]; deleted {
				continue
			}
			if finalName, moving := finalNameByID[holder.Id]; moving && finalName != holder.Name {
				continue
			}
			return fmt.Errorf("分组名称重复: %s", item.Name)
		}

		// 先删除已通过引用校验的分组，释放它们占用的唯一名称。
		for index := range deletedGroups {
			group := &deletedGroups[index]
			if tx.Migrator().HasTable(&GroupAlias{}) {
				if err := tx.Where("group_id = ?", group.Id).Delete(&GroupAlias{}).Error; err != nil {
					return fmt.Errorf("删除分组 %s 的兼容别名失败: %w", group.Name, err)
				}
			}
			if err := tx.Delete(group).Error; err != nil {
				return err
			}
		}

		// 唯一索引会让 A/B -> B/A 的逐行更新在第一步就冲突。
		// 事务内先把所有参与换名的记录移到唯一占位名，再写最终名称。
		for _, item := range prepared {
			existing, ok := existingByID[item.Id]
			if !ok || existing.Name == item.Name {
				continue
			}
			var temporaryName string
			for attempt := 0; attempt < 100; attempt++ {
				candidate := fmt.Sprintf("__group_name_swap_%d_%d_%d", item.Id, temporaryNonce, attempt)
				if _, reserved := seenNames[candidate]; reserved {
					continue
				}
				var count int64
				if err := tx.Model(&Group{}).Where("name = ?", candidate).Count(&count).Error; err != nil {
					return err
				}
				if count == 0 {
					temporaryName = candidate
					break
				}
			}
			if temporaryName == "" {
				return fmt.Errorf("无法为分组 %s 分配临时名称", existing.Name)
			}
			if err := tx.Model(&Group{}).Where("id = ?", item.Id).Update("name", temporaryName).Error; err != nil {
				return fmt.Errorf("准备修改分组 %s 的名称失败: %w", existing.Name, err)
			}
		}

		for _, item := range prepared {
			updates := map[string]interface{}{"name": item.Name, "description": item.Description, "ratio": item.Ratio, "user_selectable": item.UserSelectable, "exclusive": item.Exclusive, "status": item.Status, "updated_time": time.Now().Unix()}
			if item.Status == 0 {
				updates["status"] = GroupStatusDisabled
			}
			if err := tx.Model(&Group{}).Where("id = ?", item.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("1 = 1").Delete(&AutoGroupMember{}).Error; err != nil {
			return err
		}
		ordered := make([]GroupConfig, 0)
		for _, item := range prepared {
			if item.AutoEnabled {
				ordered = append(ordered, item)
			}
		}
		sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].AutoOrder < ordered[j].AutoOrder })
		for position, item := range ordered {
			if err := tx.Create(&AutoGroupMember{GroupId: item.Id, Position: position}).Error; err != nil {
				return err
			}
		}
		if err := writeGroupProjection(); err != nil {
			return err
		}
		return validateStoredGroupReferenceOptions(tx)
	})
	if err != nil {
		return nil, err
	}
	InvalidateExclusiveGroupSnapshot()
	cacheSummary := &TokenGroupMigrationSummary{}
	invalidateTokenGroupMigrationCaches(migratedTokenPlans, cacheSummary)
	result.CacheInvalidated = cacheSummary.CacheInvalidated
	result.CacheInvalidationFailed = cacheSummary.CacheInvalidationFailed
	result.Warning = cacheSummary.Warning
	// DB 事务成功后刷新运行时设置；旧配置镜像仍可被旧版本读取。
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
	projectionKeys := make([]string, 0, len(projection))
	for key := range projection {
		projectionKeys = append(projectionKeys, key)
	}
	projectionKeys = sortedUniqueOptionKeys(projectionKeys)
	for _, key := range projectionKeys {
		if err := updateOptionMap(key, projection[key]); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// SaveGroupConfigWithResult 保留不携带高级选项的调用契约。
func SaveGroupConfigWithResult(configs []GroupConfig, deletedIDs []int) (*GroupConfigSaveResult, error) {
	return SaveGroupConfigWithOptionsAndResult(configs, deletedIDs, nil)
}

// SaveGroupConfig 保留原有调用契约。
func SaveGroupConfig(configs []GroupConfig, deletedIDs []int) error {
	_, err := SaveGroupConfigWithResult(configs, deletedIDs)
	return err
}
