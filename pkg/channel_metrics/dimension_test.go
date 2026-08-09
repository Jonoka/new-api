package channelmetrics

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateUTF8PreservesBoundaryAndAddsStableHash(t *testing.T) {
	original := strings.Repeat("模型", 20)
	truncated := TruncateUTF8(original, 24)
	if !utf8.ValidString(truncated) {
		t.Fatalf("截断结果不是合法 UTF-8：%q", truncated)
	}
	if len(truncated) > 24 {
		t.Fatalf("截断结果长度 = %d，超过 24 字节", len(truncated))
	}
	if !strings.Contains(truncated, "~") {
		t.Fatalf("截断结果缺少哈希后缀：%q", truncated)
	}
	if TruncateUTF8(original, 24) != truncated {
		t.Fatal("相同原文的截断结果必须稳定")
	}
	if TruncateUTF8("短模型", 24) != "短模型" {
		t.Fatal("未超过上限的文本不应被修改")
	}
}

func TestDimensionHashSeparatesPresenceAndAmbiguousStrings(t *testing.T) {
	base := NewLiveSample(ScopeFinalRequest, OutcomeSuccess)
	base.RequestedModelPresent = true
	base.RequestedModel = "a|bc"
	base.Group = "d"
	first, err := DimensionFromSample(base, DefaultSnapshotLimits())
	if err != nil {
		t.Fatalf("生成首个维度失败：%v", err)
	}

	secondSample := base
	secondSample.RequestedModel = "a"
	secondSample.Group = "bc|d"
	second, err := DimensionFromSample(secondSample, DefaultSnapshotLimits())
	if err != nil {
		t.Fatalf("生成第二个维度失败：%v", err)
	}
	if DimensionHash(first) == DimensionHash(second) {
		t.Fatal("长度前缀维度编码不应产生分隔符歧义")
	}
	if len(DimensionHash(first)) != 64 || DimensionHash(first) != DimensionHash(first) {
		t.Fatal("维度哈希必须是稳定的完整 SHA-256")
	}

	emptyPresentSample := base
	emptyPresentSample.RequestedModel = ""
	emptyPresent, err := DimensionFromSample(emptyPresentSample, DefaultSnapshotLimits())
	if err != nil {
		t.Fatalf("生成显式空模型维度失败：%v", err)
	}
	absentSample := base
	absentSample.RequestedModelPresent = false
	absentSample.RequestedModel = "会被清空"
	absent, err := DimensionFromSample(absentSample, DefaultSnapshotLimits())
	if err != nil {
		t.Fatalf("生成不适用模型维度失败：%v", err)
	}
	if DimensionHash(emptyPresent) == DimensionHash(absent) {
		t.Fatal("显式空模型和不适用模型必须拥有不同维度哈希")
	}
}

func TestDimensionUsesFullModelHashDespiteSnapshotTruncation(t *testing.T) {
	base := NewLiveSample(ScopeFinalRequest, OutcomeSuccess)
	base.RequestedModelPresent = true
	base.RequestedModel = strings.Repeat("相同前缀", 20) + "甲"
	limits := DefaultSnapshotLimits()
	limits.ModelBytes = 32
	first, err := DimensionFromSample(base, limits)
	if err != nil {
		t.Fatalf("生成首个长模型维度失败：%v", err)
	}
	base.RequestedModel = strings.Repeat("相同前缀", 20) + "乙"
	second, err := DimensionFromSample(base, limits)
	if err != nil {
		t.Fatalf("生成第二个长模型维度失败：%v", err)
	}
	if first.RequestedModelHash == second.RequestedModelHash || DimensionHash(first) == DimensionHash(second) {
		t.Fatal("不同完整模型名不能因展示截断而合并")
	}
	if len(first.RequestedModel) > limits.ModelBytes || !utf8.ValidString(first.RequestedModel) {
		t.Fatalf("模型展示快照不符合限制：%q", first.RequestedModel)
	}
}
