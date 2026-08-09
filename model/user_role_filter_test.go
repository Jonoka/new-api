package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUserRoleFilterFixture(t *testing.T, user *User) {
	t.Helper()
	user.AffCode = common.GetRandomString(16)
	require.NoError(t, DB.Create(user).Error)
}

func TestSearchUsersFiltersExactRole(t *testing.T) {
	truncateTables(t)

	insertUserRoleFilterFixture(t, &User{Id: 9201, Username: "role-user", Role: common.RoleCommonUser})
	insertUserRoleFilterFixture(t, &User{Id: 9202, Username: "role-admin", Role: common.RoleAdminUser})
	insertUserRoleFilterFixture(t, &User{Id: 9203, Username: "role-root", Role: common.RoleRootUser})

	for _, tt := range []struct {
		name       string
		role       int
		expectedID int
	}{
		{name: "普通用户", role: common.RoleCommonUser, expectedID: 9201},
		{name: "管理员", role: common.RoleAdminUser, expectedID: 9202},
		{name: "超级管理员", role: common.RoleRootUser, expectedID: 9203},
	} {
		t.Run(tt.name, func(t *testing.T) {
			users, total, err := SearchUsers("", "", &tt.role, nil, 0, 10)

			require.NoError(t, err)
			assert.Equal(t, int64(1), total)
			require.Len(t, users, 1)
			assert.Equal(t, tt.expectedID, users[0].Id)
		})
	}
}
