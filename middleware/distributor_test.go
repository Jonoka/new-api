package middleware

import "testing"

func TestImageGenerationPathHelpers(t *testing.T) {
	cases := map[string]struct {
		generation bool
		task       bool
	}{
		"/v1/images/generations":                    {generation: true, task: false},
		"/v1/images/generations/task_123":           {generation: true, task: true},
		"/canvas/v1/images/generations":             {generation: true, task: false},
		"/canvas/v1/images/generations/task_123":    {generation: true, task: true},
		"/canvas/v1/images/edits/task_123":          {generation: false, task: true},
		"/api/v1/images/generations/task_123":       {generation: false, task: false},
		"/v1/images/edits":                          {generation: false, task: false},
	}

	for path, want := range cases {
		if got := isImageGenerationPath(path); got != want.generation {
			t.Fatalf("isImageGenerationPath(%q) = %v, want %v", path, got, want.generation)
		}
		if got := isImageGenerationTaskPath(path); got != want.task {
			t.Fatalf("isImageGenerationTaskPath(%q) = %v, want %v", path, got, want.task)
		}
	}
}
