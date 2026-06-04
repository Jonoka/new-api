package relay

import "testing"

func TestIsOpenAIVideoRequestURIIncludesCanvasProxy(t *testing.T) {
	cases := map[string]bool{
		"/v1/videos/task_123":                true,
		"/v1/videos/task_123/content":        true,
		"/canvas/v1/videos/task_123":         true,
		"/canvas/v1/videos/task_123/content": true,
		"/api/v1/videos/task_123":            false,
	}

	for requestURI, want := range cases {
		if got := isOpenAIVideoRequestURI(requestURI); got != want {
			t.Fatalf("isOpenAIVideoRequestURI(%q) = %v, want %v", requestURI, got, want)
		}
	}
}
