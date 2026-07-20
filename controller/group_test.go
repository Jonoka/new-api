package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func migrationTargetID(value int) *int {
	return &value
}

func TestTokenGroupMigrationRequestResolveTarget(t *testing.T) {
	tests := []struct {
		name     string
		request  TokenGroupMigrationRequest
		wantMode string
		wantID   int
		wantErr  bool
	}{
		{
			name:     "兼容旧显式请求",
			request:  TokenGroupMigrationRequest{SourceGroupID: 1, TargetGroupID: migrationTargetID(2)},
			wantMode: model.TokenGroupModeExplicit,
			wantID:   2,
		},
		{
			name: "显式声明目标模式",
			request: TokenGroupMigrationRequest{
				SourceGroupID:   1,
				TargetGroupID:   migrationTargetID(2),
				TargetGroupMode: model.TokenGroupModeExplicit,
			},
			wantMode: model.TokenGroupModeExplicit,
			wantID:   2,
		},
		{
			name:     "auto 不需要目标 ID",
			request:  TokenGroupMigrationRequest{SourceGroupID: 1, TargetGroupMode: model.TokenGroupModeAuto},
			wantMode: model.TokenGroupModeAuto,
		},
		{
			name: "auto 兼容显式零值 ID",
			request: TokenGroupMigrationRequest{
				SourceGroupID:   1,
				TargetGroupID:   migrationTargetID(0),
				TargetGroupMode: model.TokenGroupModeAuto,
			},
			wantMode: model.TokenGroupModeAuto,
		},
		{
			name:    "漏传目标 ID 不能隐式转 auto",
			request: TokenGroupMigrationRequest{SourceGroupID: 1},
			wantErr: true,
		},
		{
			name: "auto 不能同时指定实体目标",
			request: TokenGroupMigrationRequest{
				SourceGroupID:   1,
				TargetGroupID:   migrationTargetID(2),
				TargetGroupMode: model.TokenGroupModeAuto,
			},
			wantErr: true,
		},
		{
			name:    "拒绝未知目标模式",
			request: TokenGroupMigrationRequest{SourceGroupID: 1, TargetGroupMode: "unknown"},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mode, targetID, err := test.request.resolveTarget()
			if test.wantErr {
				if err == nil {
					t.Fatalf("预期返回错误，实际 mode=%q targetID=%d", mode, targetID)
				}
				return
			}
			if err != nil {
				t.Fatalf("解析迁移目标失败: %v", err)
			}
			if mode != test.wantMode || targetID != test.wantID {
				t.Fatalf("迁移目标错误: mode=%q targetID=%d", mode, targetID)
			}
		})
	}
}
