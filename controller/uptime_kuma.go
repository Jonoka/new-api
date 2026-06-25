package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

const (
	requestTimeout              = 30 * time.Second
	httpTimeout                 = 10 * time.Second
	defaultUptimeTimeWindowHour = 24
	minUptimeTimeWindowHour     = 1
	maxUptimeTimeWindowHour     = 720
	uptimeKeySuffix             = "_24"
	apiStatusPath               = "/api/status-page/"
	apiHeartbeatPath            = "/api/status-page/heartbeat/"
)

type Monitor struct {
	Name   string  `json:"name"`
	Uptime float64 `json:"uptime"`
	Status int     `json:"status"`
	Group  string  `json:"group,omitempty"`
}

type UptimeGroupResult struct {
	CategoryName    string    `json:"categoryName"`
	Monitors        []Monitor `json:"monitors"`
	EmbedUrl        string    `json:"embedUrl,omitempty"`
	TimeWindowHours int       `json:"timeWindowHours"`
	TimeWindowLabel string    `json:"timeWindowLabel"`
}

func getAndDecode(ctx context.Context, client *http.Client, url string, dest interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("non-200 status")
	}

	return common.DecodeJson(resp.Body, dest)
}

func normalizeUptimeTimeWindowHours(hours int) int {
	if hours < minUptimeTimeWindowHour {
		return minUptimeTimeWindowHour
	}
	if hours > maxUptimeTimeWindowHour {
		return maxUptimeTimeWindowHour
	}
	return hours
}

func getUptimeTimeWindowHours(groupConfig map[string]interface{}) int {
	value, exists := groupConfig["timeWindowHours"]
	if !exists || value == nil {
		return defaultUptimeTimeWindowHour
	}

	var hours int
	switch v := value.(type) {
	case int:
		hours = v
	case int8:
		hours = int(v)
	case int16:
		hours = int(v)
	case int32:
		hours = int(v)
	case int64:
		hours = int(v)
	case uint:
		hours = int(v)
	case uint8:
		hours = int(v)
	case uint16:
		hours = int(v)
	case uint32:
		hours = int(v)
	case uint64:
		hours = int(v)
	case float32:
		hours = int(v)
	case float64:
		hours = int(v)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return defaultUptimeTimeWindowHour
		}
		hours = parsed
	default:
		return defaultUptimeTimeWindowHour
	}

	return normalizeUptimeTimeWindowHours(hours)
}

func formatUptimeTimeWindowLabel(hours int) string {
	return fmt.Sprintf("%dH", normalizeUptimeTimeWindowHours(hours))
}

func fetchGroupData(ctx context.Context, client *http.Client, groupConfig map[string]interface{}) UptimeGroupResult {
	url, _ := groupConfig["url"].(string)
	slug, _ := groupConfig["slug"].(string)
	embedUrl, _ := groupConfig["embedUrl"].(string)
	categoryName, _ := groupConfig["categoryName"].(string)
	timeWindowHours := getUptimeTimeWindowHours(groupConfig)

	result := UptimeGroupResult{
		CategoryName:    categoryName,
		Monitors:        []Monitor{},
		EmbedUrl:        embedUrl,
		TimeWindowHours: timeWindowHours,
		TimeWindowLabel: formatUptimeTimeWindowLabel(timeWindowHours),
	}

	if embedUrl != "" {
		return result
	}

	if url == "" || slug == "" {
		return result
	}

	baseURL := strings.TrimSuffix(url, "/")

	var statusData struct {
		PublicGroupList []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			MonitorList []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"monitorList"`
		} `json:"publicGroupList"`
	}

	var heartbeatData struct {
		HeartbeatList map[string][]struct {
			Status int `json:"status"`
		} `json:"heartbeatList"`
		UptimeList map[string]float64 `json:"uptimeList"`
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return getAndDecode(gCtx, client, baseURL+apiStatusPath+slug, &statusData)
	})
	g.Go(func() error {
		return getAndDecode(gCtx, client, baseURL+apiHeartbeatPath+slug, &heartbeatData)
	})

	if g.Wait() != nil {
		return result
	}

	for _, pg := range statusData.PublicGroupList {
		if len(pg.MonitorList) == 0 {
			continue
		}

		for _, m := range pg.MonitorList {
			monitor := Monitor{
				Name:  m.Name,
				Group: pg.Name,
			}

			monitorID := strconv.Itoa(m.ID)

			if uptime, exists := heartbeatData.UptimeList[monitorID+uptimeKeySuffix]; exists {
				monitor.Uptime = uptime
			}

			if heartbeats, exists := heartbeatData.HeartbeatList[monitorID]; exists && len(heartbeats) > 0 {
				monitor.Status = heartbeats[0].Status
			}

			result.Monitors = append(result.Monitors, monitor)
		}
	}

	return result
}

func GetUptimeKumaStatus(c *gin.Context) {
	groups := console_setting.GetUptimeKumaGroups()
	if len(groups) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": []UptimeGroupResult{}})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	client := &http.Client{Timeout: httpTimeout}
	results := make([]UptimeGroupResult, len(groups))

	g, gCtx := errgroup.WithContext(ctx)
	for i, group := range groups {
		i, group := i, group
		g.Go(func() error {
			results[i] = fetchGroupData(gCtx, client, group)
			return nil
		})
	}

	g.Wait()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": results})
}
