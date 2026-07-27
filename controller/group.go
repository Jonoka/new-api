package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

// GetGroupDetails 返回稳定 ID、兼容标识和可编辑显示属性。
// 旧的 GetGroups 接口继续只返回字符串，避免已有客户端解析失败。
func GetGroupDetails(c *gin.Context) {
	groups, err := model.GetAllGroups(true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "",
		"data":       groups,
		"auto_group": setting.GetAutoGroupConfig(),
	})
}

type groupConfigUpdateRequest struct {
	model.GroupConfig
	Exclusive *bool `json:"exclusive"`
}

func (r groupConfigUpdateRequest) toModel() model.GroupConfig {
	config := r.GroupConfig
	if r.Exclusive == nil {
		config.ExclusiveOmitted = true
		return config
	}
	config.Exclusive = *r.Exclusive
	return config
}

type GroupDetailsUpdateRequest struct {
	Groups        []groupConfigUpdateRequest `json:"groups"`
	DeletedIDs    []int                      `json:"deleted_ids"`
	OptionUpdates map[string]string          `json:"option_updates,omitempty"`
	AutoGroup     *setting.AutoGroupConfig   `json:"auto_group,omitempty"`
}

func (r GroupDetailsUpdateRequest) modelGroups() []model.GroupConfig {
	groups := make([]model.GroupConfig, 0, len(r.Groups))
	for _, group := range r.Groups {
		groups = append(groups, group.toModel())
	}
	return groups
}

type TokenGroupMigrationRequest struct {
	SourceGroupID   int    `json:"source_group_id"`
	TargetGroupID   *int   `json:"target_group_id,omitempty"`
	TargetGroupMode string `json:"target_group_mode,omitempty"`
}

type GroupCodeMigrationRequest struct {
	Confirm bool `json:"confirm"`
}

// PreviewGroupCodeMigration 返回旧分组 code 跟随稳定 ID 迁移的影响和阻塞项。
func PreviewGroupCodeMigration(c *gin.Context) {
	summary, err := model.PreviewGroupCodeMigration()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

// MigrateGroupCodes 执行管理员明确确认的旧分组 code 迁移。
func MigrateGroupCodes(c *gin.Context) {
	var request GroupCodeMigrationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "分组标识迁移参数格式错误")
		return
	}
	if !request.Confirm {
		common.ApiErrorMsg(c, "执行分组标识迁移前必须明确确认")
		return
	}
	summary, err := model.MigrateLegacyGroupCodesToIDs()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(
		c.GetInt("id"),
		model.LogTypeManage,
		fmt.Sprintf(
			"迁移旧分组标识：共迁移 %d 个分组，影响 %d 个渠道、%d 个令牌、%d 个用户、%d 条能力记录；缓存清理成功 %d 项、失败 %d 项",
			len(summary.Groups),
			summary.AffectedChannels,
			summary.AffectedTokens,
			summary.AffectedUsers,
			summary.AffectedAbilities,
			summary.CacheInvalidated,
			summary.CacheInvalidationFailed,
		),
	)
	common.ApiSuccess(c, summary)
}

func decodeTokenGroupMigrationRequest(c *gin.Context) (*TokenGroupMigrationRequest, bool) {
	var request TokenGroupMigrationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "令牌分组迁移参数格式错误")
		return nil, false
	}
	return &request, true
}

func (request *TokenGroupMigrationRequest) resolveTarget() (string, int, error) {
	if request == nil {
		return "", 0, fmt.Errorf("令牌分组迁移参数不能为空")
	}
	mode := strings.ToLower(strings.TrimSpace(request.TargetGroupMode))
	if mode == "" {
		mode = model.TokenGroupModeExplicit
	}
	switch mode {
	case model.TokenGroupModeExplicit:
		if request.TargetGroupID == nil || *request.TargetGroupID <= 0 {
			return "", 0, fmt.Errorf("显式迁移必须指定大于 0 的目标分组 ID")
		}
		return mode, *request.TargetGroupID, nil
	case model.TokenGroupModeAuto:
		if request.TargetGroupID != nil && *request.TargetGroupID != 0 {
			return "", 0, fmt.Errorf("迁移到 auto 时不能指定目标分组 ID")
		}
		return mode, 0, nil
	default:
		return "", 0, fmt.Errorf("不支持的令牌分组迁移目标模式: %s", mode)
	}
}

// PreviewTokenGroupMigration 返回令牌分组迁移会影响的数据量。
func PreviewTokenGroupMigration(c *gin.Context) {
	request, ok := decodeTokenGroupMigrationRequest(c)
	if !ok {
		return
	}
	targetMode, targetGroupID, err := request.resolveTarget()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var summary *model.TokenGroupMigrationSummary
	if targetMode == model.TokenGroupModeAuto {
		summary, err = model.PreviewTokenGroupMigrationToAuto(request.SourceGroupID)
	} else {
		summary, err = model.PreviewTokenGroupMigration(request.SourceGroupID, targetGroupID)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

// MigrateTokenGroup 将明确绑定源分组的令牌迁移到目标分组。
func MigrateTokenGroup(c *gin.Context) {
	request, ok := decodeTokenGroupMigrationRequest(c)
	if !ok {
		return
	}
	targetMode, targetGroupID, err := request.resolveTarget()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var summary *model.TokenGroupMigrationSummary
	if targetMode == model.TokenGroupModeAuto {
		summary, err = model.MigrateTokenGroupToAuto(request.SourceGroupID)
	} else {
		summary, err = model.MigrateTokenGroup(request.SourceGroupID, targetGroupID)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	targetDescription := fmt.Sprintf("%s（ID %d）", summary.TargetGroup.Name, summary.TargetGroup.Id)
	detailDescription := fmt.Sprintf("其中 %d 个令牌合并了重复分组", summary.DeduplicatedTokens)
	if summary.TargetGroupMode == model.TokenGroupModeAuto {
		targetDescription = "自动选择（auto）"
		detailDescription = fmt.Sprintf(
			"其中 %d 个多分组令牌已移除其他全部分组和倍率保护",
			summary.MultiGroupTokens,
		)
	}
	model.RecordLog(
		c.GetInt("id"),
		model.LogTypeManage,
		fmt.Sprintf(
			"迁移令牌分组：%s（ID %d）到 %s，共迁移 %d 个有效令牌、清理 %d 个已删除令牌，%s；缓存清理成功 %d 个、失败 %d 个",
			summary.SourceGroup.Name,
			summary.SourceGroup.Id,
			targetDescription,
			summary.MigratedTokens,
			summary.CleanedDeletedTokens,
			detailDescription,
			summary.CacheInvalidated,
			summary.CacheInvalidationFailed,
		),
	)
	common.ApiSuccess(c, summary)
}

// UpdateGroupDetails 批量保存分组显示属性和自动分组顺序。
func UpdateGroupDetails(c *gin.Context) {
	var request GroupDetailsUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "分组配置格式错误")
		return
	}
	result, err := model.SaveGroupConfigWithOptionsAndAutoConfigResult(
		request.modelGroups(),
		request.DeletedIDs,
		request.OptionUpdates,
		request.AutoGroup,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	groups, err := model.GetAllGroups(true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if result.MigratedTokens > 0 || result.CleanedDeletedTokens > 0 {
		model.RecordLog(
			c.GetInt("id"),
			model.LogTypeManage,
			fmt.Sprintf(
				"保存分组配置：删除分组时将 %d 个有效令牌切换为自动选择，并清理 %d 个已删除令牌的历史分组；缓存清理成功 %d 个、失败 %d 个",
				result.MigratedTokens,
				result.CleanedDeletedTokens,
				result.CacheInvalidated,
				result.CacheInvalidationFailed,
			),
		)
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    result.Warning,
		"data":       groups,
		"auto_group": setting.GetAutoGroupConfig(),
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			groupID := 0
			groupCode := groupName
			groupNameForDisplay := groupName
			groupExclusive := false
			if group, err := model.GetGroupByCodeOrAlias(groupName); err == nil {
				groupID = group.Id
				groupCode = group.Code
				groupNameForDisplay = group.Name
				groupExclusive = group.Exclusive
			}
			usableGroups[groupName] = map[string]interface{}{
				"id":        groupID,
				"code":      groupCode,
				"name":      groupNameForDisplay,
				"ratio":     service.GetUserGroupRatio(userGroup, groupName),
				"desc":      desc,
				"exclusive": groupExclusive,
			}
		}
	}
	if desc, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]interface{}{
			"id":    0,
			"code":  "auto",
			"name":  "自动选择",
			"ratio": "自动",
			"desc":  desc,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}
