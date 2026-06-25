package controller

import (
	"encoding/json"
	"testing"
)

func TestGetSidebarModulesAdminStatusValuePreservesCustomItems(t *testing.T) {
	raw := `{"chat":{"enabled":true,"playground":true,"chat":true},"customItems":[{"id":"canvas","title":"无限画布","url":"/canvas","enabled":true,"section":"chat","icon":"Brush","order":10}]}`

	value := getSidebarModulesAdminStatusValue(raw)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		t.Fatalf("status value is not valid json: %v", err)
	}
	items, ok := parsed["customItems"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("customItems not preserved: %#v", parsed["customItems"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("custom item has unexpected shape: %#v", items[0])
	}
	if item["id"] != "canvas" || item["url"] != "/canvas" {
		t.Fatalf("custom item was changed unexpectedly: %#v", item)
	}
}
