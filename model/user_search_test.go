package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUserSearchFixture(t *testing.T, user *User) {
	t.Helper()
	if user.AffCode == "" {
		user.AffCode = common.GetRandomString(16)
	}
	require.NoError(t, DB.Create(user).Error)
}

func TestSearchUsersNumericKeywordPrioritizesExactID(t *testing.T) {
	truncateTables(t)

	insertUserSearchFixture(t, &User{
		Id:          6281,
		Username:    "target-user",
		DisplayName: "目标用户",
		Email:       "target@example.com",
	})
	insertUserSearchFixture(t, &User{
		Id:          7001,
		Username:    "user-6281",
		DisplayName: "6281-display",
		Email:       "6281@example.com",
	})

	users, total, err := SearchUsers("6281", "", nil, nil, 0, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, users, 2)
	assert.Equal(t, 6281, users[0].Id)
	assert.Equal(t, 7001, users[1].Id)

	firstPage, firstPageTotal, err := SearchUsers("6281", "", nil, nil, 0, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), firstPageTotal)
	require.Len(t, firstPage, 1)
	assert.Equal(t, 6281, firstPage[0].Id)
}

func TestSearchUsersTextKeywordKeepsFuzzyMatching(t *testing.T) {
	truncateTables(t)

	insertUserSearchFixture(t, &User{
		Id:          8101,
		Username:    "searchable-user",
		DisplayName: "普通用户",
		Email:       "first@example.com",
	})
	insertUserSearchFixture(t, &User{
		Id:          8102,
		Username:    "another-user",
		DisplayName: "searchable display",
		Email:       "second@example.com",
	})

	users, total, err := SearchUsers("searchable", "", nil, nil, 0, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, users, 2)
	assert.Equal(t, 8102, users[0].Id)
	assert.Equal(t, 8101, users[1].Id)
}

func TestSearchUsersSupportsExplicitSearchType(t *testing.T) {
	truncateTables(t)

	insertUserSearchFixture(t, &User{
		Id:       6281,
		Username: "target-user",
	})
	insertUserSearchFixture(t, &User{
		Id:          7001,
		Username:    "user-6281",
		DisplayName: "6281-display",
		Email:       "6281@example.com",
	})

	idUsers, idTotal, err := SearchUsers("6281", "", nil, nil, 0, 10, "id")
	require.NoError(t, err)
	assert.Equal(t, int64(1), idTotal)
	require.Len(t, idUsers, 1)
	assert.Equal(t, 6281, idUsers[0].Id)

	usernameUsers, usernameTotal, err := SearchUsers("6281", "", nil, nil, 0, 10, "username")
	require.NoError(t, err)
	assert.Equal(t, int64(1), usernameTotal)
	require.Len(t, usernameUsers, 1)
	assert.Equal(t, 7001, usernameUsers[0].Id)

	invalidIDUsers, invalidIDTotal, err := SearchUsers("user-6281", "", nil, nil, 0, 10, "id")
	require.NoError(t, err)
	assert.Zero(t, invalidIDTotal)
	assert.Empty(t, invalidIDUsers)

	emptyIDUsers, emptyIDTotal, err := SearchUsers("", "", nil, nil, 0, 10, "id")
	require.NoError(t, err)
	assert.Equal(t, int64(2), emptyIDTotal)
	assert.Len(t, emptyIDUsers, 2)
}
