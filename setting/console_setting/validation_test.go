package console_setting

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestValidateUptimeKumaGroupsAllowsEmbedUrlWithoutKumaUrlAndSlug(t *testing.T) {
	err := validateUptimeKumaGroups(`[{"id":1,"categoryName":"ApiPanelWatch","url":"","slug":"","embedUrl":"https://status.example.com/embed/status?channelId=1"}]`)
	if err != nil {
		t.Fatalf("validateUptimeKumaGroups returned error: %v", err)
	}
}

func TestValidateUptimeKumaGroupsRejectsInvalidTimeWindowHours(t *testing.T) {
	err := validateUptimeKumaGroups(`[{"id":1,"categoryName":"API","url":"https://status.example.com","slug":"api","timeWindowHours":721}]`)
	if err == nil {
		t.Fatalf("validateUptimeKumaGroups returned nil, want error")
	}
}

func TestValidateAnnouncementsCountsExtraByCharacters(t *testing.T) {
	tests := []struct {
		name        string
		extra       string
		wantErrText string
	}{
		{
			name:  "二百个中文字符允许保存",
			extra: strings.Repeat("明", 200),
		},
		{
			name:        "二百零一个中文字符拒绝保存",
			extra:       strings.Repeat("明", 201),
			wantErrText: "第1个公告的说明长度不能超过200字符",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			announcements, err := common.Marshal([]map[string]interface{}{
				{
					"content":     "公告",
					"publishDate": "2026-07-27T00:00:00Z",
					"type":        "default",
					"extra":       tt.extra,
				},
			})
			if err != nil {
				t.Fatalf("序列化测试公告失败：%v", err)
			}

			err = validateAnnouncements(string(announcements))
			if tt.wantErrText == "" {
				if err != nil {
					t.Fatalf("期望校验通过，实际返回错误：%v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("期望返回错误 %q，实际校验通过", tt.wantErrText)
			}
			if err.Error() != tt.wantErrText {
				t.Fatalf("错误信息不一致：期望 %q，实际 %q", tt.wantErrText, err.Error())
			}
		})
	}
}

func TestValidateAnnouncementsCountsContentByCharacters(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErrText string
	}{
		{
			name:    "五百个中文字符允许保存",
			content: strings.Repeat("公", 500),
		},
		{
			name:        "五百零一个中文字符拒绝保存",
			content:     strings.Repeat("公", 501),
			wantErrText: "第1个公告的内容长度不能超过500字符",
		},
		{
			name:    "五百个ASCII字符允许保存",
			content: strings.Repeat("a", 500),
		},
		{
			name:        "五百零一个ASCII字符拒绝保存",
			content:     strings.Repeat("a", 501),
			wantErrText: "第1个公告的内容长度不能超过500字符",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			announcements, err := common.Marshal([]map[string]interface{}{
				{
					"content":     tt.content,
					"publishDate": "2026-07-27T00:00:00Z",
					"type":        "default",
				},
			})
			if err != nil {
				t.Fatalf("序列化测试公告失败：%v", err)
			}

			err = validateAnnouncements(string(announcements))
			if tt.wantErrText == "" {
				if err != nil {
					t.Fatalf("期望校验通过，实际返回错误：%v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("期望返回错误 %q，实际校验通过", tt.wantErrText)
			}
			if err.Error() != tt.wantErrText {
				t.Fatalf("错误信息不一致：期望 %q，实际 %q", tt.wantErrText, err.Error())
			}
		})
	}
}
