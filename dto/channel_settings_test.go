package dto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageTasksSubmitPathValidation(t *testing.T) {
	tests := map[string]string{
		"":                           DefaultImageTasksEndpoint,
		"v1/custom/tasks":            "/v1/custom/tasks",
		"/v1/custom/tasks/":          "/v1/custom/tasks",
		"https://evil.invalid/tasks": DefaultImageTasksEndpoint,
		"//evil.invalid/tasks":       DefaultImageTasksEndpoint,
		"/custom/tasks?tenant=x":     DefaultImageTasksEndpoint,
		"/custom/tasks#fragment":     DefaultImageTasksEndpoint,
		"/custom/../tasks":           DefaultImageTasksEndpoint,
		"/custom/%2e%2e/tasks":       DefaultImageTasksEndpoint,
		"/custom/%252e%252e/tasks":   DefaultImageTasksEndpoint,
		"/custom/tasks%3Ftenant=x":   DefaultImageTasksEndpoint,
		"/custom/tasks%23fragment":   DefaultImageTasksEndpoint,
		"/custom/tasks%0aextra":      DefaultImageTasksEndpoint,
		`/custom\tasks`:               DefaultImageTasksEndpoint,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got := (ChannelOtherSettings{ImageTasksEndpoint: input}).ImageTasksSubmitPath()
			require.Equal(t, want, got)
		})
	}
}
