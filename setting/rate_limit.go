package setting

import (
	"fmt"
	"math"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

type RateLimitCounts [2]int

type UserGroupRateLimit struct {
	Global *RateLimitCounts           `json:"global,omitempty"`
	Groups map[string]RateLimitCounts `json:"groups,omitempty"`
}

var ModelRequestRateLimitEnabled = false
var ModelRequestRateLimitDurationMinutes = 1
var ModelRequestRateLimitCount = 0
var ModelRequestRateLimitSuccessCount = 1000
var ModelRequestRateLimitGroup = map[string]RateLimitCounts{}
var ModelRequestRateLimitUserGroup = map[string]UserGroupRateLimit{}
var ModelRequestRateLimitMutex sync.RWMutex

func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitGroup)
	if err != nil {
		common.SysLog("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	next := make(map[string]RateLimitCounts)
	if err := common.UnmarshalJsonStr(jsonStr, &next); err != nil {
		return err
	}
	for group, limits := range next {
		if err := validateRateLimitCounts(fmt.Sprintf("group %s", group), limits); err != nil {
			return err
		}
	}

	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()

	ModelRequestRateLimitGroup = next
	return nil
}

func GetGroupRateLimit(group string) (totalCount, successCount int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	if ModelRequestRateLimitGroup == nil {
		return 0, 0, false
	}

	limits, found := ModelRequestRateLimitGroup[group]
	if !found {
		return 0, 0, false
	}
	return limits[0], limits[1], true
}

func ModelRequestRateLimitUserGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitUserGroup)
	if err != nil {
		common.SysLog("error marshalling model request rate limit user group: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelRequestRateLimitUserGroupByJSONString(jsonStr string) error {
	next := make(map[string]UserGroupRateLimit)
	if err := common.UnmarshalJsonStr(jsonStr, &next); err != nil {
		return err
	}
	if err := validateModelRequestRateLimitUserGroup(next); err != nil {
		return err
	}

	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()

	ModelRequestRateLimitUserGroup = next
	return nil
}

func GetUserGroupGlobalRateLimit(userGroup string) (totalCount, successCount int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	limits, found := ModelRequestRateLimitUserGroup[userGroup]
	if !found || limits.Global == nil {
		return 0, 0, false
	}
	return (*limits.Global)[0], (*limits.Global)[1], true
}

func GetUserGroupRateLimit(userGroup, group string) (totalCount, successCount int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	limits, found := ModelRequestRateLimitUserGroup[userGroup]
	if !found || limits.Groups == nil {
		return 0, 0, false
	}
	groupLimits, found := limits.Groups[group]
	if !found {
		return 0, 0, false
	}
	return groupLimits[0], groupLimits[1], true
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	checkModelRequestRateLimitGroup := make(map[string]RateLimitCounts)
	if err := common.UnmarshalJsonStr(jsonStr, &checkModelRequestRateLimitGroup); err != nil {
		return err
	}
	for group, limits := range checkModelRequestRateLimitGroup {
		if err := validateRateLimitCounts(fmt.Sprintf("group %s", group), limits); err != nil {
			return err
		}
	}

	return nil
}

func CheckModelRequestRateLimitUserGroup(jsonStr string) error {
	checkModelRequestRateLimitUserGroup := make(map[string]UserGroupRateLimit)
	if err := common.UnmarshalJsonStr(jsonStr, &checkModelRequestRateLimitUserGroup); err != nil {
		return err
	}
	return validateModelRequestRateLimitUserGroup(checkModelRequestRateLimitUserGroup)
}

func validateModelRequestRateLimitUserGroup(config map[string]UserGroupRateLimit) error {
	for userGroup, limits := range config {
		if userGroup == "" {
			return fmt.Errorf("user group is empty")
		}
		if limits.Global != nil {
			if err := validateRateLimitCounts(fmt.Sprintf("user group %s global", userGroup), *limits.Global); err != nil {
				return err
			}
		}
		for group, groupLimits := range limits.Groups {
			if group == "" {
				return fmt.Errorf("user group %s request group is empty", userGroup)
			}
			if err := validateRateLimitCounts(fmt.Sprintf("user group %s request group %s", userGroup, group), groupLimits); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRateLimitCounts(label string, limits RateLimitCounts) error {
	if limits[0] < 0 {
		return fmt.Errorf("%s has negative rate limit total value: [%d, %d]", label, limits[0], limits[1])
	}
	if limits[1] < 1 {
		return fmt.Errorf("%s has invalid success rate limit value: [%d, %d]", label, limits[0], limits[1])
	}
	if limits[0] > math.MaxInt32 || limits[1] > math.MaxInt32 {
		return fmt.Errorf("%s [%d, %d] has max rate limits value 2147483647", label, limits[0], limits[1])
	}
	return nil
}
