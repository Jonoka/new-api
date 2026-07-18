package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

func buildMaskedTokenResponse(token *model.Token) *model.Token {
	if token == nil {
		return nil
	}
	maskedToken := *token
	maskedToken.Key = token.GetMaskedKey()
	return &maskedToken
}

func buildMaskedTokenResponses(tokens []*model.Token) []*model.Token {
	maskedTokens := make([]*model.Token, 0, len(tokens))
	for _, token := range tokens {
		maskedTokens = append(maskedTokens, buildMaskedTokenResponse(token))
	}
	return maskedTokens
}

func bindTokenJSON(c *gin.Context, token *model.Token) (map[string]any, error) {
	raw := make(map[string]any)
	if err := common.UnmarshalBodyReusable(c, &raw); err != nil {
		return nil, err
	}
	if err := common.UnmarshalBodyReusable(c, token); err != nil {
		return nil, err
	}
	return raw, nil
}

func resolveTokenGroupForCreate(requestGroup string, userGroup string) string {
	requestGroup = strings.TrimSpace(requestGroup)
	if requestGroup != "" {
		return requestGroup
	}
	if setting.DefaultUseAutoGroup {
		return "auto"
	}
	return strings.TrimSpace(userGroup)
}

// validateTokenGroups 校验多分组令牌的分组字段。
// 规则：auto 不能与其他分组混用；分组不能重复；分组名不能为空。
func validateTokenGroups(group string) error {
	if group == "" || group == "auto" {
		return nil
	}
	if !strings.Contains(group, ",") {
		return nil // 单分组不额外校验
	}
	groups := strings.Split(group, ",")
	seen := make(map[string]bool)
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g == "" {
			return errors.New("分组名不能为空")
		}
		if g == "auto" {
			return errors.New("auto 不能与其他分组混用")
		}
		if seen[g] {
			return errors.New("分组不能重复: " + g)
		}
		seen[g] = true
	}
	return nil
}

func parseTokenGroupSet(group string) map[string]bool {
	result := make(map[string]bool)
	for _, g := range strings.Split(group, ",") {
		g = strings.TrimSpace(g)
		if g != "" {
			result[g] = true
		}
	}
	return result
}

func normalizeTokenGroupRatioLimits(group string, limitsJSON string) (string, error) {
	limitsJSON = strings.TrimSpace(limitsJSON)
	if limitsJSON == "" {
		return "", nil
	}
	limits := make(map[string]float64)
	if err := common.UnmarshalJsonStr(limitsJSON, &limits); err != nil {
		return "", fmt.Errorf("倍率保护配置格式错误: %w", err)
	}
	if len(limits) == 0 {
		return "", nil
	}
	groupSet := parseTokenGroupSet(group)
	if len(groupSet) == 0 {
		return "", errors.New("设置倍率保护前请先选择令牌分组")
	}
	if groupSet["auto"] {
		return "", errors.New("auto 分组不支持设置倍率保护，请改用明确分组")
	}
	normalized := make(map[string]float64, len(limits))
	for g, ratio := range limits {
		g = strings.TrimSpace(g)
		if g == "" {
			return "", errors.New("倍率保护分组名不能为空")
		}
		if ratio <= 0 {
			return "", fmt.Errorf("分组 %s 的倍率保护必须大于 0", g)
		}
		if _, ok := groupSet[g]; !ok {
			return "", fmt.Errorf("分组 %s 未在令牌分组中选择，不能设置倍率保护", g)
		}
		normalized[g] = ratio
	}
	if len(normalized) == 0 {
		return "", nil
	}
	data, err := common.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func GetAllTokens(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(userId)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func SearchTokens(c *gin.Context) {
	userId := c.GetInt("id")
	keyword := c.Query("keyword")
	token := c.Query("token")

	pageInfo := common.GetPageQuery(c)

	tokens, total, err := model.SearchUserTokens(userId, keyword, token, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func GetToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildMaskedTokenResponse(token))
}

func GetTokenKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"key": token.GetFullKey(),
	})
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"object":          "credit_summary",
		"total_granted":   token.RemainQuota,
		"total_used":      0, // not supported currently
		"total_available": token.RemainQuota,
		"expires_at":      expiredAt * 1000,
	})
}

func GetTokenUsage(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "No Authorization header",
		})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid Bearer token",
		})
		return
	}
	tokenKey := parts[1]

	token, err := model.GetTokenByKey(strings.TrimPrefix(tokenKey, "sk-"), false)
	if err != nil {
		common.SysError("failed to get token by key: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgTokenGetInfoFailed)
		return
	}

	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    true,
		"message": "ok",
		"data": gin.H{
			"object":               "token_usage",
			"name":                 token.Name,
			"total_granted":        token.RemainQuota + token.UsedQuota,
			"total_used":           token.UsedQuota,
			"total_available":      token.RemainQuota,
			"unlimited_quota":      token.UnlimitedQuota,
			"model_limits":         token.GetModelLimitsMap(),
			"model_limits_enabled": token.ModelLimitsEnabled,
			"expires_at":           expiredAt,
		},
	})
}

func AddToken(c *gin.Context) {
	token := model.Token{}
	_, err := bindTokenJSON(c, &token)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	// 非无限额度时，检查额度值是否超出有效范围
	if !token.UnlimitedQuota {
		if token.RemainQuota < 0 {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
			return
		}
		maxQuotaValue := int((1000000000 * common.QuotaPerUnit))
		if token.RemainQuota > maxQuotaValue {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxQuotaValue})
			return
		}
	}
	// 检查用户令牌数量是否已达上限
	maxTokens := operation_setting.GetMaxUserTokens()
	count, err := model.CountUserTokens(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if int(count) >= maxTokens {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("已达到最大令牌数量限制 (%d)", maxTokens),
		})
		return
	}
	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgTokenGenerateFailed)
		common.SysLog("failed to generate token key: " + err.Error())
		return
	}
	resolvedGroup := resolveTokenGroupForCreate(token.Group, c.GetString("group"))
	selection := model.Token{Group: resolvedGroup, GroupMode: token.GroupMode, GroupIds: token.GroupIds}
	if err := model.PrepareTokenGroupBindings(model.DB, &selection); err != nil {
		common.ApiError(c, err)
		return
	}
	resolvedGroup = selection.Group
	if err := validateTokenGroups(resolvedGroup); err != nil {
		common.ApiError(c, err)
		return
	}
	normalizedGroupRatioLimits, err := normalizeTokenGroupRatioLimits(resolvedGroup, token.GroupRatioLimits)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cleanToken := model.Token{
		UserId:             c.GetInt("id"),
		Name:               token.Name,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        token.ExpiredTime,
		RemainQuota:        token.RemainQuota,
		UnlimitedQuota:     token.UnlimitedQuota,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
		AllowIps:           token.AllowIps,
		Group:              resolvedGroup,
		GroupMode:          selection.GroupMode,
		GroupIds:           selection.GroupIds,
		GroupDetails:       selection.GroupDetails,
		GroupRatioLimits:   normalizedGroupRatioLimits,
		CrossGroupRetry:    token.CrossGroupRetry,
	}
	err = cleanToken.Insert()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DeleteToken(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	err := model.DeleteTokenById(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateToken(c *gin.Context) {
	userId := c.GetInt("id")
	statusOnly := c.Query("status_only")
	token := model.Token{}
	rawFields, err := bindTokenJSON(c, &token)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_, groupRatioLimitsProvided := rawFields["group_ratio_limits"]
	_, groupProvided := rawFields["group"]
	_, groupIdsProvided := rawFields["group_ids"]
	_, groupModeProvided := rawFields["group_mode"]
	if rawFields["group"] == nil {
		groupProvided = false
	}
	if rawFields["group_ids"] == nil {
		groupIdsProvided = false
	}
	if rawFields["group_mode"] == nil {
		groupModeProvided = false
	}
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	if !token.UnlimitedQuota {
		if token.RemainQuota < 0 {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
			return
		}
		maxQuotaValue := int((1000000000 * common.QuotaPerUnit))
		if token.RemainQuota > maxQuotaValue {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxQuotaValue})
			return
		}
	}
	cleanToken, err := model.GetTokenByIds(token.Id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if token.Status == common.TokenStatusEnabled {
		if cleanToken.Status == common.TokenStatusExpired && cleanToken.ExpiredTime <= common.GetTimestamp() && cleanToken.ExpiredTime != -1 {
			common.ApiErrorI18n(c, i18n.MsgTokenExpiredCannotEnable)
			return
		}
		if cleanToken.Status == common.TokenStatusExhausted && cleanToken.RemainQuota <= 0 && !cleanToken.UnlimitedQuota {
			common.ApiErrorI18n(c, i18n.MsgTokenExhaustedCannotEable)
			return
		}
	}
	if statusOnly != "" {
		cleanToken.Status = token.Status
	} else {
		groupSelectionProvided := groupProvided || groupIdsProvided || groupModeProvided
		selection := model.Token{
			Id:           cleanToken.Id,
			Group:        cleanToken.Group,
			GroupMode:    cleanToken.GroupMode,
			GroupIds:     cleanToken.GroupIds,
			GroupDetails: cleanToken.GroupDetails,
		}
		if groupSelectionProvided {
			selection.Group = token.Group
			selection.GroupMode = token.GroupMode
			selection.GroupIds = token.GroupIds
			selection.GroupDetails = nil
			if !groupIdsProvided {
				selection.GroupIds = nil
			}
			if !groupModeProvided {
				selection.GroupMode = ""
			}
			if err := model.PrepareTokenGroupBindingsForUpdate(model.DB, &selection); err != nil {
				common.ApiError(c, err)
				return
			}
		}
		if err := validateTokenGroups(selection.Group); err != nil {
			common.ApiError(c, err)
			return
		}
		normalizedGroupRatioLimits := ""
		if groupRatioLimitsProvided {
			normalizedGroupRatioLimits, err = normalizeTokenGroupRatioLimits(selection.Group, token.GroupRatioLimits)
			if err != nil {
				common.ApiError(c, err)
				return
			}
		}
		// If you add more fields, please also update token.Update()
		cleanToken.Name = token.Name
		cleanToken.ExpiredTime = token.ExpiredTime
		cleanToken.RemainQuota = token.RemainQuota
		cleanToken.UnlimitedQuota = token.UnlimitedQuota
		cleanToken.ModelLimitsEnabled = token.ModelLimitsEnabled
		cleanToken.ModelLimits = token.ModelLimits
		cleanToken.AllowIps = token.AllowIps
		cleanToken.Group = selection.Group
		cleanToken.GroupMode = selection.GroupMode
		cleanToken.GroupIds = selection.GroupIds
		cleanToken.GroupDetails = selection.GroupDetails
		if groupRatioLimitsProvided {
			cleanToken.GroupRatioLimits = normalizedGroupRatioLimits
		} else if groupSelectionProvided && selection.GroupMode != model.TokenGroupModeExplicit {
			cleanToken.GroupRatioLimits = ""
		}
		cleanToken.CrossGroupRetry = token.CrossGroupRetry
	}
	err = cleanToken.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(cleanToken),
	})
}

type TokenBatch struct {
	Ids []int `json:"ids"`
}

func DeleteTokenBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userId := c.GetInt("id")
	count, err := model.BatchDeleteTokens(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}

func GetTokenKeysBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(tokenBatch.Ids) > 100 {
		common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": 100})
		return
	}
	userId := c.GetInt("id")
	tokens, err := model.GetTokenKeysByIds(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	keysMap := make(map[int]string)
	for _, t := range tokens {
		keysMap[t.Id] = t.GetFullKey()
	}
	common.ApiSuccess(c, gin.H{"keys": keysMap})
}
