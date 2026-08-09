package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateNotificationStringUsesCharacterLength(t *testing.T) {
	require.NoError(t, validateNotificationString(strings.Repeat("中", 128), "名称", 128))
	require.ErrorContains(t, validateNotificationString(strings.Repeat("中", 129), "名称", 128), "最多允许 128 个字符")
}
