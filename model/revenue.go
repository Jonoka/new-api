package model

import (
	"sort"
)

// RevenueDataPoint represents a single time bucket of revenue data.
type RevenueDataPoint struct {
	Timestamp       int64   `json:"timestamp"`
	OnlineMoney     float64 `json:"online_money"`
	RedemptionQuota int64   `json:"redemption_quota"`
	OnlineCount     int     `json:"online_count"`
	RedemptionCount int     `json:"redemption_count"`
}

// RevenueSummary holds the aggregated totals for the queried time range.
type RevenueSummary struct {
	TotalOnlineMoney     float64 `json:"total_online_money"`
	TotalRedemptionQuota int64   `json:"total_redemption_quota"`
	TotalOnlineCount     int     `json:"total_online_count"`
	TotalRedemptionCount int     `json:"total_redemption_count"`
}

// RevenueStatsResponse is the complete response for the revenue statistics API.
type RevenueStatsResponse struct {
	Summary    RevenueSummary     `json:"summary"`
	DataPoints []RevenueDataPoint `json:"data_points"`
}

// revenueRow is an internal struct for scanning raw SQL aggregation results.
type revenueRow struct {
	Bucket int64   `gorm:"column:bucket"`
	Money  float64 `gorm:"column:money"`
	Quota  int64   `gorm:"column:quota"`
	Cnt    int     `gorm:"column:cnt"`
}

// GetRevenueStats aggregates revenue data across TopUp, SubscriptionOrder,
// and RedemptionUsage within the given time range.
func GetRevenueStats(startTime, endTime int64, granularity string) (*RevenueStatsResponse, error) {
	bucketSeconds := int64(86400) // default: day
	if granularity == "hour" {
		bucketSeconds = 3600
	}

	dataMap := make(map[int64]*RevenueDataPoint)

	// --- Query 1: TopUp (online recharge) ---
	if err := queryTopUpRevenue(dataMap, bucketSeconds, startTime, endTime); err != nil {
		return nil, err
	}

	// Note: subscription orders are already recorded in top_ups when completed,
	// so we do NOT query subscription_orders separately to avoid double-counting.

	// --- Query 2: RedemptionUsage (redemption code) ---
	if err := queryRedemptionRevenue(dataMap, bucketSeconds, startTime, endTime); err != nil {
		return nil, err
	}

	// Build response
	summary := RevenueSummary{}
	dataPoints := make([]RevenueDataPoint, 0, len(dataMap))
	for _, dp := range dataMap {
		summary.TotalOnlineMoney += dp.OnlineMoney
		summary.TotalRedemptionQuota += dp.RedemptionQuota
		summary.TotalOnlineCount += dp.OnlineCount
		summary.TotalRedemptionCount += dp.RedemptionCount
		dataPoints = append(dataPoints, *dp)
	}

	sort.Slice(dataPoints, func(i, j int) bool {
		return dataPoints[i].Timestamp < dataPoints[j].Timestamp
	})

	return &RevenueStatsResponse{
		Summary:    summary,
		DataPoints: dataPoints,
	}, nil
}

func getOrCreateDataPoint(dataMap map[int64]*RevenueDataPoint, bucket int64) *RevenueDataPoint {
	dp, ok := dataMap[bucket]
	if !ok {
		dp = &RevenueDataPoint{Timestamp: bucket}
		dataMap[bucket] = dp
	}
	return dp
}

func queryTopUpRevenue(dataMap map[int64]*RevenueDataPoint, bucketSeconds, startTime, endTime int64) error {
	var rows []revenueRow
	err := DB.Raw(
		"SELECT (complete_time / ? * ?) AS bucket, SUM(actual_money) AS money, COUNT(*) AS cnt "+
			"FROM top_ups "+
			"WHERE status = 'success' AND payment_provider != 'balance' "+
			"AND complete_time >= ? AND complete_time <= ? "+
			"GROUP BY bucket ORDER BY bucket",
		bucketSeconds, bucketSeconds, startTime, endTime,
	).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, r := range rows {
		dp := getOrCreateDataPoint(dataMap, r.Bucket)
		dp.OnlineMoney += r.Money
		dp.OnlineCount += r.Cnt
	}
	return nil
}

func queryRedemptionRevenue(dataMap map[int64]*RevenueDataPoint, bucketSeconds, startTime, endTime int64) error {
	var rows []revenueRow
	err := DB.Raw(
		"SELECT (ru.created_time / ? * ?) AS bucket, SUM(r.quota) AS quota, COUNT(*) AS cnt "+
			"FROM redemption_usages ru "+
			"LEFT JOIN redemptions r ON r.id = ru.redemption_id "+
			"WHERE ru.created_time >= ? AND ru.created_time <= ? "+
			"GROUP BY bucket ORDER BY bucket",
		bucketSeconds, bucketSeconds, startTime, endTime,
	).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, r := range rows {
		dp := getOrCreateDataPoint(dataMap, r.Bucket)
		dp.RedemptionQuota += r.Quota
		dp.RedemptionCount += r.Cnt
	}
	return nil
}
