package channelmetrics

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"strings"
	"unicode/utf8"
)

const (
	// DimensionVersion 修改哈希字段布局时必须递增。
	DimensionVersion = 1
	shortHashLength  = 10
)

// SnapshotLimits 限制进入热桶和持久层的展示文本长度。
// 哈希始终基于未截断原文，因此相同前缀不会被错误合并。
type SnapshotLimits struct {
	ModelBytes       int
	ChannelNameBytes int
	GroupBytes       int
	ErrorStageBytes  int
}

func DefaultSnapshotLimits() SnapshotLimits {
	return SnapshotLimits{
		ModelBytes:       128,
		ChannelNameBytes: 128,
		GroupBytes:       64,
		ErrorStageBytes:  32,
	}
}

func (l SnapshotLimits) normalized() SnapshotLimits {
	defaults := DefaultSnapshotLimits()
	if l.ModelBytes <= 0 {
		l.ModelBytes = defaults.ModelBytes
	}
	if l.ChannelNameBytes <= 0 {
		l.ChannelNameBytes = defaults.ChannelNameBytes
	}
	if l.GroupBytes <= 0 {
		l.GroupBytes = defaults.GroupBytes
	}
	if l.ErrorStageBytes <= 0 {
		l.ErrorStageBytes = defaults.ErrorStageBytes
	}
	return l
}

// Dimension 是持久化安全且可比较的完整维度。
// 展示快照经过 UTF-8 安全截断，Hash 字段保留完整原文身份。
type Dimension struct {
	Version int   `json:"dimension_version"`
	Scope   Scope `json:"metric_scope"`

	ChannelPresent      bool   `json:"channel_present"`
	ChannelID           int    `json:"channel_id"`
	ChannelNameSnapshot string `json:"channel_name_snapshot"`
	ChannelNameHash     string `json:"channel_name_hash"`
	ChannelType         int    `json:"channel_type"`

	RequestedModelPresent bool   `json:"requested_model_present"`
	RequestedModel        string `json:"requested_model"`
	RequestedModelHash    string `json:"requested_model_hash"`
	UpstreamModelPresent  bool   `json:"upstream_model_present"`
	UpstreamModel         string `json:"upstream_model"`
	UpstreamModelHash     string `json:"upstream_model_hash"`
	Group                 string `json:"group"`
	GroupHash             string `json:"group_hash"`

	TrafficSource TrafficSource `json:"traffic_source"`
	DataOrigin    DataOrigin    `json:"data_origin"`
	Stream        bool          `json:"stream"`
	Outcome       Outcome       `json:"outcome"`
	ErrorStage    ErrorStage    `json:"error_stage"`
	FailureOwner  FailureOwner  `json:"failure_owner"`

	QualityEligible bool `json:"quality_eligible"`
	PartialResponse bool `json:"partial_response"`

	ClientStatus     StatusCode `json:"client_status"`
	UpstreamStatus   StatusCode `json:"upstream_status"`
	NormalizedStatus StatusCode `json:"normalized_status"`

	// Overflowed 标记该桶已失去高基数维度，只能用于不完整汇总。
	Overflowed bool `json:"overflowed"`
}

// DimensionFromSample 生成可直接用于热桶和持久化的维度快照。
func DimensionFromSample(sample Sample, limits SnapshotLimits) (Dimension, error) {
	sample = normalizeSample(sample)
	if err := validateSample(sample); err != nil {
		return Dimension{}, err
	}
	limits = limits.normalized()
	if len(string(sample.ErrorStage)) > limits.ErrorStageBytes {
		return Dimension{}, invalidSample("error_stage 超过 %d 字节上限", limits.ErrorStageBytes)
	}

	dimension := Dimension{
		Version:               DimensionVersion,
		Scope:                 sample.Scope,
		ChannelPresent:        sample.ChannelPresent,
		ChannelID:             sample.ChannelID,
		ChannelType:           sample.ChannelType,
		RequestedModelPresent: sample.RequestedModelPresent,
		UpstreamModelPresent:  sample.UpstreamModelPresent,
		TrafficSource:         sample.TrafficSource,
		DataOrigin:            sample.DataOrigin,
		Stream:                sample.Stream,
		Outcome:               sample.Outcome,
		ErrorStage:            sample.ErrorStage,
		FailureOwner:          sample.FailureOwner,
		QualityEligible:       sample.QualityEligible,
		PartialResponse:       sample.PartialResponse,
		ClientStatus:          sample.ClientStatus,
		UpstreamStatus:        sample.UpstreamStatus,
		NormalizedStatus:      sample.NormalizedStatus,
	}
	if sample.ChannelPresent {
		dimension.ChannelNameSnapshot = TruncateUTF8(sample.ChannelNameSnapshot, limits.ChannelNameBytes)
		dimension.ChannelNameHash = SHA256String(sample.ChannelNameSnapshot)
	}
	if sample.RequestedModelPresent {
		dimension.RequestedModel = TruncateUTF8(sample.RequestedModel, limits.ModelBytes)
		dimension.RequestedModelHash = SHA256String(sample.RequestedModel)
	}
	if sample.UpstreamModelPresent {
		dimension.UpstreamModel = TruncateUTF8(sample.UpstreamModel, limits.ModelBytes)
		dimension.UpstreamModelHash = SHA256String(sample.UpstreamModel)
	}
	dimension.Group = TruncateUTF8(sample.Group, limits.GroupBytes)
	dimension.GroupHash = SHA256String(sample.Group)
	return dimension, nil
}

// DimensionHash 使用版本前缀、固定字段顺序、固定宽整数和长度前缀字符串
// 生成完整 SHA-256。展示快照不参与身份计算，避免截断长度调整改变指标身份。
func DimensionHash(dimension Dimension) string {
	digest := sha256.New()
	encoder := dimensionEncoder{target: digest}
	encoder.string("channel_metrics_dimension")
	encoder.int64(int64(dimension.Version))
	encoder.string(string(dimension.Scope))
	encoder.boolean(dimension.ChannelPresent)
	encoder.int64(int64(dimension.ChannelID))
	encoder.string(dimension.ChannelNameHash)
	encoder.int64(int64(dimension.ChannelType))
	encoder.boolean(dimension.RequestedModelPresent)
	encoder.string(dimension.RequestedModelHash)
	encoder.boolean(dimension.UpstreamModelPresent)
	encoder.string(dimension.UpstreamModelHash)
	encoder.string(dimension.GroupHash)
	encoder.string(string(dimension.TrafficSource))
	encoder.string(string(dimension.DataOrigin))
	encoder.boolean(dimension.Stream)
	encoder.string(string(dimension.Outcome))
	encoder.string(string(dimension.ErrorStage))
	encoder.string(string(dimension.FailureOwner))
	encoder.boolean(dimension.QualityEligible)
	encoder.boolean(dimension.PartialResponse)
	encoder.status(dimension.ClientStatus)
	encoder.status(dimension.UpstreamStatus)
	encoder.status(dimension.NormalizedStatus)
	encoder.boolean(dimension.Overflowed)
	return hex.EncodeToString(digest.Sum(nil))
}

// SHA256String 返回原始字符串的完整小写 SHA-256 十六进制值。
func SHA256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// TruncateUTF8 按字节上限截断，并为被截断文本附加稳定短哈希后缀。
// 返回值始终是合法 UTF-8，且不会超过 maxBytes。
func TruncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= maxBytes {
		return value
	}

	suffix := "~" + SHA256String(value)[:shortHashLength]
	if maxBytes <= len(suffix) {
		return suffix[:maxBytes]
	}
	prefixBytes := maxBytes - len(suffix)
	for prefixBytes > 0 && !utf8.ValidString(value[:prefixBytes]) {
		prefixBytes--
	}
	return value[:prefixBytes] + suffix
}

func overflowDimension(source Dimension) Dimension {
	overflowHash := SHA256String("__other__")
	return Dimension{
		Version:             DimensionVersion,
		Scope:               source.Scope,
		ChannelNameSnapshot: "__other__",
		ChannelNameHash:     overflowHash,
		RequestedModel:      "__other__",
		RequestedModelHash:  overflowHash,
		UpstreamModel:       "__other__",
		UpstreamModelHash:   overflowHash,
		Group:               "__other__",
		GroupHash:           overflowHash,
		TrafficSource:       source.TrafficSource,
		DataOrigin:          source.DataOrigin,
		Outcome:             source.Outcome,
		FailureOwner:        source.FailureOwner,
		QualityEligible:     source.QualityEligible,
		Overflowed:          true,
	}
}

type dimensionEncoder struct {
	target hash.Hash
}

func (e dimensionEncoder) string(value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = e.target.Write(length[:])
	_, _ = e.target.Write([]byte(value))
}

func (e dimensionEncoder) int64(value int64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(value))
	_, _ = e.target.Write(encoded[:])
}

func (e dimensionEncoder) boolean(value bool) {
	if value {
		_, _ = e.target.Write([]byte{1})
		return
	}
	_, _ = e.target.Write([]byte{0})
}

func (e dimensionEncoder) status(status StatusCode) {
	e.boolean(status.Present)
	e.int64(int64(status.Code))
}
