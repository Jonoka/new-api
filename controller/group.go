package controller

import (
	"net/http"

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
	common.ApiSuccess(c, groups)
}

type GroupDetailsUpdateRequest struct {
	Groups     []model.GroupConfig `json:"groups"`
	DeletedIDs []int               `json:"deleted_ids"`
}

// UpdateGroupDetails 批量保存分组显示属性和自动分组顺序。
func UpdateGroupDetails(c *gin.Context) {
	var request GroupDetailsUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "分组配置格式错误")
		return
	}
	if err := model.SaveGroupConfig(request.Groups, request.DeletedIDs); err != nil {
		common.ApiError(c, err)
		return
	}
	GetGroupDetails(c)
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
			if group, err := model.GetGroupByCodeOrAlias(groupName); err == nil {
				groupID = group.Id
				groupCode = group.Code
				groupNameForDisplay = group.Name
			}
			usableGroups[groupName] = map[string]interface{}{
				"id":    groupID,
				"code":  groupCode,
				"name":  groupNameForDisplay,
				"ratio": service.GetUserGroupRatio(userGroup, groupName),
				"desc":  desc,
			}
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]interface{}{
			"id":    0,
			"code":  "auto",
			"name":  "自动选择",
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}
