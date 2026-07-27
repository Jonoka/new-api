package model

import "testing"

func TestGetPerfMetricsMergesCurrentCodeAndHistoricalAliasIdentity(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Group{}, &GroupAlias{}, &PerfMetric{}); err != nil {
		t.Fatalf("迁移性能指标测试表失败: %v", err)
	}
	defaultGroup := Group{Code: "default", Name: "默认", Ratio: 1, Status: GroupStatusActive}
	if err := db.Create(&defaultGroup).Error; err != nil {
		t.Fatal(err)
	}
	group := Group{Code: "2", Name: "特价", Ratio: 1, Status: GroupStatusActive}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&GroupAlias{Alias: "group_2", GroupId: group.Id}).Error; err != nil {
		t.Fatal(err)
	}
	metrics := []PerfMetric{
		{ModelName: "gpt-test", Group: "group_2", BucketTs: 100, RequestCount: 2},
		{ModelName: "gpt-test", Group: group.Code, BucketTs: 100, RequestCount: 3},
	}
	if err := db.Create(&metrics).Error; err != nil {
		t.Fatal(err)
	}

	rows, err := GetPerfMetrics("gpt-test", group.Code, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("性能指标行数 = %d，期望同时读取当前 code 和历史 alias", len(rows))
	}
	for _, row := range rows {
		if row.Group != group.Code {
			t.Fatalf("历史指标未归并到当前 code：%q", row.Group)
		}
	}

	allRows, err := GetPerfMetrics("gpt-test", "", 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range allRows {
		if row.Group != group.Code {
			t.Fatalf("未指定筛选时历史指标未归并到当前 code：%q", row.Group)
		}
	}
}
