package middleware

import "testing"

func TestFormatDistributorGroupForMessage(t *testing.T) {
	groupNames := map[string]string{
		"group_1":      "普通分组",
		"group_2":      "图片分组",
		"legacy_image": "图片分组",
		"blank_name":   "  ",
	}
	tests := []struct {
		name        string
		usingGroup  string
		selectGroup string
		groupNames  map[string]string
		want        string
	}{
		{"普通分组代码", "group_2", "group_2", groupNames, "图片分组"},
		{"普通分组别名", "legacy_image", "legacy_image", groupNames, "图片分组"},
		{"普通分组缺少映射", "missing", "missing", groupNames, "missing"},
		{"普通分组空白名称", "blank_name", "blank_name", groupNames, "blank_name"},
		{"自动分组选中代码", "auto", "group_2", groupNames, "auto(图片分组)"},
		{"自动分组选中别名", "auto", "legacy_image", groupNames, "auto(图片分组)"},
		{"自动分组未选中具体分组", "auto", "auto", groupNames, "auto"},
		{"多分组选中代码", "group_1,group_2", "group_2", groupNames, "multi(图片分组)"},
		{"多分组选中别名", "group_1,legacy_image", "legacy_image", groupNames, "multi(图片分组)"},
		{"多分组无可用渠道", "group_1,group_2", "group_1,group_2", groupNames, "multi(普通分组,图片分组)"},
		{"映射失败时普通分组回退", "group_2", "group_2", nil, "group_2"},
		{"映射失败时自动分组回退", "auto", "group_2", nil, "auto(group_2)"},
		{"映射失败时多分组回退", "group_1,group_2", "group_1,group_2", nil, "multi(group_1,group_2)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDistributorGroupForMessage(tt.usingGroup, tt.selectGroup, tt.groupNames)
			if got != tt.want {
				t.Fatalf("formatDistributorGroupForMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
