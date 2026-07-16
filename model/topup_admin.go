package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// AdminTopUp 是管理员充值账单视图，在订单信息之外补充当前用户名。
// 用户名不冗余写入充值订单，避免用户改名后账单仍显示旧名称。
type AdminTopUp struct {
	*TopUp
	Username string `json:"username"`
}

type topUpUsernameRow struct {
	Id       int
	Username string
}

// GetAdminTopUps 获取全平台充值记录，并批量补充用户名。
func GetAdminTopUps(pageInfo *common.PageInfo) (topups []*AdminTopUp, total int64, err error) {
	items, total, err := GetAllTopUps(pageInfo)
	if err != nil {
		return nil, 0, err
	}
	result, err := attachTopUpUsernames(items)
	return result, total, err
}

// SearchAdminTopUps 支持管理员按订单号、用户名或精确用户 ID 查询充值记录。
func SearchAdminTopUps(keyword string, pageInfo *common.PageInfo) (topups []*AdminTopUp, total int64, err error) {
	keyword = strings.TrimSpace(keyword)
	pattern, err := sanitizeLikePattern(keyword)
	if err != nil {
		return nil, 0, err
	}

	query := DB.Model(&TopUp{})
	userSubQuery := DB.Model(&User{}).
		Select("id").
		Where("username LIKE ? ESCAPE '!'", pattern)
	query = query.Where("trade_no LIKE ? ESCAPE '!' OR user_id IN (?)", pattern, userSubQuery)
	if userId, parseErr := strconv.Atoi(keyword); parseErr == nil && userId > 0 {
		query = query.Or("user_id = ?", userId)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		common.SysError("failed to count admin search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	var items []*TopUp
	if err = query.Order("id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&items).Error; err != nil {
		common.SysError("failed to search admin topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	result, err := attachTopUpUsernames(items)
	return result, total, err
}

func attachTopUpUsernames(topups []*TopUp) ([]*AdminTopUp, error) {
	result := make([]*AdminTopUp, 0, len(topups))
	if len(topups) == 0 {
		return result, nil
	}

	userIds := make([]int, 0, len(topups))
	seen := make(map[int]struct{}, len(topups))
	for _, topUp := range topups {
		if topUp == nil {
			continue
		}
		if _, exists := seen[topUp.UserId]; !exists {
			seen[topUp.UserId] = struct{}{}
			userIds = append(userIds, topUp.UserId)
		}
	}

	var rows []topUpUsernameRow
	if len(userIds) > 0 {
		if err := DB.Model(&User{}).
			Select("id, username").
			Where("id IN ?", userIds).
			Scan(&rows).Error; err != nil {
			return nil, err
		}
	}
	usernameById := make(map[int]string, len(rows))
	for _, row := range rows {
		usernameById[row.Id] = row.Username
	}

	for _, topUp := range topups {
		if topUp == nil {
			continue
		}
		result = append(result, &AdminTopUp{
			TopUp:    topUp,
			Username: usernameById[topUp.UserId],
		})
	}
	return result, nil
}
