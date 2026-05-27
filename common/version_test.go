package common

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRuntimeVersion(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		envVersion  string
		fileContent string
		gitVersion  string
		want        string
	}{
		{
			name:       "环境变量优先",
			envVersion: "v9.9.9",
			gitVersion: "v1.0.0-rc.10",
			want:       "v9.9.9",
		},
		{
			name:        "非空 VERSION 文件优先于 git",
			fileContent: "v1.2.3\n",
			gitVersion:  "v1.0.0-rc.10",
			want:        "v1.2.3",
		},
		{
			name:       "VERSION 文件为空时使用 git 描述",
			gitVersion: "v1.0.0-rc.10",
			want:       "v1.0.0-rc.10",
		},
		{
			name: "没有可用版本时保留当前默认值",
			want: Version,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			versionFile := filepath.Join(tempDir, tt.name+".VERSION")
			if err := os.WriteFile(versionFile, []byte(tt.fileContent), 0o600); err != nil {
				t.Fatalf("write version file: %v", err)
			}

			got := resolveRuntimeVersion(tt.envVersion, versionFile, func() (string, error) {
				if tt.gitVersion == "" {
					return "", os.ErrNotExist
				}
				return tt.gitVersion, nil
			})

			if got != tt.want {
				t.Fatalf("resolveRuntimeVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
