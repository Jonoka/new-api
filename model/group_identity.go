package model

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Group 是路由/用户分组的稳定身份。
//
// Code 是兼容旧 API 和内部运行时的不可变标识；Name 是只用于展示的可变名称。
// 这样修改名称不会改变渠道、令牌、订阅或历史数据所指向的分组身份。
type Group struct {
	Id             int     `json:"id"`
	Code           string  `json:"code" gorm:"size:64;not null;uniqueIndex:idx_groups_code"`
	Name           string  `json:"name" gorm:"size:128;not null;uniqueIndex:idx_groups_name"`
	Description    string  `json:"description,omitempty" gorm:"type:text"`
	Ratio          float64 `json:"ratio" gorm:"default:1"`
	UserSelectable bool    `json:"user_selectable" gorm:"default:false"`
	Status         int     `json:"status" gorm:"default:1;index"`
	CreatedTime    int64   `json:"created_time" gorm:"bigint"`
	UpdatedTime    int64   `json:"updated_time" gorm:"bigint"`

	// AutoEnabled/AutoOrder 是自动分组关联表的 API 投影，不直接存储在 groups 表。
	AutoEnabled bool `json:"auto_enabled" gorm:"-"`
	AutoOrder   int  `json:"auto_order" gorm:"-"`
}

func (Group) TableName() string { return "groups" }

// GroupAlias 为将来修改 Code 保留兼容入口。当前管理 API 不允许直接修改 Code，
// 但解析层可以优先查询该表，从而支持平滑迁移旧客户端。
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
	Status         int     `json:"status"`
	AutoEnabled    bool    `json:"auto_enabled"`
	AutoOrder      int     `json:"auto_order"`
}

func (g *Group) ToConfig(autoMembers map[int]AutoGroupMember) GroupConfig {
	config := GroupConfig{
		Id:             g.Id,
		Code:           g.Code,
		Name:           g.Name,
		Description:    g.Description,
		Ratio:          g.Ratio,
		UserSelectable: g.UserSelectable,
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
	query := DB.Model(&Group{}).Order("id ASC")
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
		if code == "" {
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

func GetGroupById(id int) (*Group, error) {
	var group Group
	if err := DB.First(&group, "id = ?", id).Error; err != nil {
		return nil, err
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
	if code == "" {
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
			if _, exists := identifierSet[alias]; exists {
				continue
			}
			identifiers = append(identifiers, alias)
			identifierSet[alias] = struct{}{}
		}
	}
	return identifiers, identifierSet, nil
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

// ResolveGroupIdsByCodes 将旧 API 的字符串分组解析为稳定 ID，并保留输入顺序。
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
	if raw := readOptionValue(options, "GroupGroupRatio"); raw != "" {
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
	query := tx.Model(&Group{}).Order("id ASC")
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
	return tx.Model(&Option{}).Where("key = ?", key).Update("value", value).Error
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
	identifiers []string,
	identifierSet map[string]struct{},
) (int64, error) {
	if !hasModelColumns(tx, model, fieldName) {
		return 0, nil
	}
	if len(identifiers) == 0 {
		return 0, nil
	}

	condition := commonGroupCol + " LIKE ? ESCAPE '!'"
	query := tx.Unscoped().Model(model).Where(condition, legacyGroupSubstringPattern(identifiers[0]))
	for _, identifier := range identifiers[1:] {
		query = query.Or(condition, legacyGroupSubstringPattern(identifier))
	}
	var values []string
	if err := query.Pluck("group", &values).Error; err != nil {
		return 0, err
	}
	var count int64
	for _, value := range values {
		if containsLegacyGroupIdentifier(value, identifierSet) {
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
		model     interface{}
		fieldName string
	}{
		{model: &Channel{}, fieldName: "Group"},
		{model: &Token{}, fieldName: "Group"},
		{model: &User{}, fieldName: "Group"},
	}
	for _, check := range legacyChecks {
		count, err := countLegacyGroupReferences(
			tx,
			check.model,
			check.fieldName,
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

// SaveGroupConfig 保存分组显示属性和自动分组顺序，并同步旧配置镜像。
func SaveGroupConfig(configs []GroupConfig, deletedIDs []int) error {
	if len(configs) == 0 && len(deletedIDs) == 0 {
		return errors.New("分组配置不能为空")
	}
	projection := map[string]string{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		seenCodes := make(map[string]struct{}, len(configs))
		seenIDs := make(map[int]struct{}, len(configs))
		for _, item := range configs {
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
			if item.Id > 0 {
				if _, ok := seenIDs[item.Id]; ok {
					return fmt.Errorf("分组 ID 重复: %d", item.Id)
				}
				seenIDs[item.Id] = struct{}{}
			}
			if item.Ratio < 0 {
				return fmt.Errorf("分组 %s 的倍率不能小于 0", code)
			}
			if item.Id == 0 {
				group := Group{Code: code, Name: name, Description: item.Description, Ratio: item.Ratio, UserSelectable: item.UserSelectable, Status: item.Status, CreatedTime: time.Now().Unix(), UpdatedTime: time.Now().Unix()}
				if group.Ratio == 0 {
					group.Ratio = 1
				}
				if group.Status == 0 {
					group.Status = GroupStatusActive
				}
				if err := tx.Create(&group).Error; err != nil {
					return err
				}
				item.Id = group.Id
				continue
			}
			var existing Group
			if err := tx.First(&existing, "id = ?", item.Id).Error; err != nil {
				return err
			}
			if existing.Code != code {
				return fmt.Errorf("分组 %d 的 code 不允许修改", item.Id)
			}
			updates := map[string]interface{}{"name": name, "description": item.Description, "ratio": item.Ratio, "user_selectable": item.UserSelectable, "status": item.Status, "updated_time": time.Now().Unix()}
			if item.Status == 0 {
				updates["status"] = GroupStatusDisabled
			}
			if err := tx.Model(&Group{}).Where("id = ?", item.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		for _, id := range deletedIDs {
			if id <= 0 {
				continue
			}
			var group Group
			if err := tx.First(&group, "id = ?", id).Error; err != nil {
				return err
			}
			if group.Code == "default" {
				return errors.New("default 分组不能删除")
			}
			referenceCount, err := groupBusinessReferenceCount(tx, &group)
			if err != nil {
				return err
			}
			if referenceCount > 0 {
				return fmt.Errorf("分组 %s 仍被业务数据引用，不能删除", group.Name)
			}
			if tx.Migrator().HasTable(&GroupAlias{}) {
				if err := tx.Where("group_id = ?", group.Id).Delete(&GroupAlias{}).Error; err != nil {
					return fmt.Errorf("删除分组 %s 的兼容别名失败: %w", group.Name, err)
				}
			}
			if err := tx.Delete(&group).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("1 = 1").Delete(&AutoGroupMember{}).Error; err != nil {
			return err
		}
		ordered := make([]GroupConfig, 0)
		for _, item := range configs {
			if item.AutoEnabled {
				ordered = append(ordered, item)
			}
		}
		sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].AutoOrder < ordered[j].AutoOrder })
		for position, item := range ordered {
			id := item.Id
			if id == 0 {
				var group Group
				if err := tx.Where("code = ?", item.Code).First(&group).Error; err != nil {
					return err
				}
				id = group.Id
			}
			if err := tx.Create(&AutoGroupMember{GroupId: id, Position: position}).Error; err != nil {
				return err
			}
		}
		var err error
		projection, err = buildGroupOptionProjection(tx)
		if err != nil {
			return err
		}
		for key, value := range projection {
			if err := upsertOption(tx, key, value); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// DB 事务成功后刷新运行时设置；旧配置镜像仍可被旧版本读取。
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
	for key, value := range projection {
		if err := updateOptionMap(key, value); err != nil {
			return err
		}
	}
	return nil
}
