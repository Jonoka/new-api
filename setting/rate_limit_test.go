package setting

import (
	"strings"
	"testing"
)

func TestUpdateModelRequestRateLimitGroupByJSONStringKeepsPreviousValueOnInvalidJSON(t *testing.T) {
	originalGroup := ModelRequestRateLimitGroup
	defer func() {
		ModelRequestRateLimitMutex.Lock()
		ModelRequestRateLimitGroup = originalGroup
		ModelRequestRateLimitMutex.Unlock()
	}()

	if err := UpdateModelRequestRateLimitGroupByJSONString(`{"codex":[0,2000]}`); err != nil {
		t.Fatalf("expected valid request group limit to update: %v", err)
	}

	if err := UpdateModelRequestRateLimitGroupByJSONString(`{"broken":`); err == nil {
		t.Fatal("expected invalid JSON to return an error")
	}

	total, success, found := GetGroupRateLimit("codex")
	if !found {
		t.Fatal("expected previous request group limit to be kept")
	}
	if total != 0 || success != 2000 {
		t.Fatalf("expected codex limit [0,2000], got [%d,%d]", total, success)
	}
}

func TestModelRequestRateLimitUserGroupConfig(t *testing.T) {
	originalUserGroup := ModelRequestRateLimitUserGroup
	defer func() {
		ModelRequestRateLimitMutex.Lock()
		ModelRequestRateLimitUserGroup = originalUserGroup
		ModelRequestRateLimitMutex.Unlock()
	}()

	raw := `{
		"vip": {
			"global": [0, 2000],
			"groups": {
				"codex": [0, 5000],
				"default": [100, 1000]
			}
		}
	}`

	if err := CheckModelRequestRateLimitUserGroup(raw); err != nil {
		t.Fatalf("expected valid user group limit JSON: %v", err)
	}
	if err := UpdateModelRequestRateLimitUserGroupByJSONString(raw); err != nil {
		t.Fatalf("expected user group limit update to succeed: %v", err)
	}

	total, success, found := GetUserGroupGlobalRateLimit("vip")
	if !found {
		t.Fatal("expected vip global limit to be found")
	}
	if total != 0 || success != 2000 {
		t.Fatalf("expected vip global limit [0,2000], got [%d,%d]", total, success)
	}

	total, success, found = GetUserGroupRateLimit("vip", "codex")
	if !found {
		t.Fatal("expected vip/codex limit to be found")
	}
	if total != 0 || success != 5000 {
		t.Fatalf("expected vip/codex limit [0,5000], got [%d,%d]", total, success)
	}

	if _, _, found = GetUserGroupRateLimit("vip", "missing"); found {
		t.Fatal("did not expect missing request group limit to be found")
	}
}

func TestCheckModelRequestRateLimitUserGroupRejectsInvalidLimits(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "empty user group",
			raw:  `{"":{"global":[0,100]}}`,
			want: "user group is empty",
		},
		{
			name: "negative total",
			raw:  `{"vip":{"global":[-1,100]}}`,
			want: "negative rate limit",
		},
		{
			name: "zero success",
			raw:  `{"vip":{"groups":{"codex":[0,0]}}}`,
			want: "success",
		},
		{
			name: "empty request group",
			raw:  `{"vip":{"groups":{"":[0,100]}}}`,
			want: "request group is empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckModelRequestRateLimitUserGroup(tc.raw)
			if err == nil {
				t.Fatal("expected invalid user group rate limit to return an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error to contain %q, got %q", tc.want, err.Error())
			}
		})
	}
}
