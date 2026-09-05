package controller

import (
	"net/http"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type imageMetricStatusWriter struct {
	gin.ResponseWriter
	status int
}

func (w imageMetricStatusWriter) Status() int { return w.status }

func deferImageMetricFinalization(c *gin.Context, info *relaycommon.RelayInfo) {
	if info.TaskSubmissionTaskRowID > 0 {
		service.MarkChannelMetricTaskRequest(c)
	}
	metricContext := c.Copy()
	if info.AttachImageMetricUsage == nil {
		info.AttachImageMetricUsage = func(quota int, err error) {
			service.AttachChannelMetricUsageAfterSettlement(metricContext, service.ChannelMetricUsage{}, quota, err)
		}
	}
	info.FinalizeImageMetrics = func(err *types.NewAPIError, status int) {
		metricContext.Writer = imageMetricStatusWriter{ResponseWriter: metricContext.Writer, status: status}
		service.FinishChannelMetricAttempt(metricContext, info, err, false, "")
		service.FinishChannelMetricRequest(metricContext, info, err)
	}
}

func finishImageMetricFinalization(info *relaycommon.RelayInfo, err error, status int) {
	if info == nil || info.FinalizeImageMetrics == nil {
		return
	}
	if info.AttachImageMetricUsage != nil {
		info.AttachImageMetricUsage(info.PriceData.Quota, err)
	}
	var apiErr *types.NewAPIError
	if err != nil {
		if typed, ok := err.(*types.NewAPIError); ok {
			apiErr = typed
		} else {
			apiErr = types.NewErrorWithStatusCode(err, types.ErrorCodeUpdateDataError, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
		}
	}
	info.FinalizeImageMetrics(apiErr, status)
	info.FinalizeImageMetrics, info.AttachImageMetricUsage = nil, nil
}
