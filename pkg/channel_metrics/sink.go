package channelmetrics

import "context"

// Sink 只负责持久化不可变增量批次。实现必须以 MetricBatch.ID 幂等去重。
type Sink interface {
	Flush(ctx context.Context, batch MetricBatch) error
}

type SinkFunc func(ctx context.Context, batch MetricBatch) error

func (f SinkFunc) Flush(ctx context.Context, batch MetricBatch) error {
	return f(ctx, batch)
}
