package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchGroupDataUsesConfiguredTimeWindowHours(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status-page/api":
			fmt.Fprint(w, `{"publicGroupList":[{"id":1,"name":"Claude","monitorList":[{"id":7,"name":"Claude API"}]}]}`)
		case "/api/status-page/heartbeat/api":
			fmt.Fprint(w, `{"heartbeatList":{"7":[{"status":1}]},"uptimeList":{"7_1":0.91,"7_24":0.99}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result := fetchGroupData(context.Background(), server.Client(), map[string]interface{}{
		"categoryName":    "Claude",
		"url":             server.URL,
		"slug":            "api",
		"timeWindowHours": float64(1),
	})

	if len(result.Monitors) != 1 {
		t.Fatalf("Monitors length = %d", len(result.Monitors))
	}
	if result.Monitors[0].Uptime != 0.91 {
		t.Fatalf("Uptime = %v, want 0.91", result.Monitors[0].Uptime)
	}
	if result.TimeWindowHours != 1 {
		t.Fatalf("TimeWindowHours = %d, want 1", result.TimeWindowHours)
	}
	if result.TimeWindowLabel != "1H" {
		t.Fatalf("TimeWindowLabel = %q, want 1H", result.TimeWindowLabel)
	}
}

func TestFetchGroupDataDefaultsToTwentyFourHourWindow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status-page/api":
			fmt.Fprint(w, `{"publicGroupList":[{"id":1,"name":"OpenAI","monitorList":[{"id":8,"name":"OpenAI API"}]}]}`)
		case "/api/status-page/heartbeat/api":
			fmt.Fprint(w, `{"heartbeatList":{"8":[{"status":1}]},"uptimeList":{"8_1":0.91,"8_24":0.98}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result := fetchGroupData(context.Background(), server.Client(), map[string]interface{}{
		"categoryName": "OpenAI",
		"url":          server.URL,
		"slug":         "api",
	})

	if len(result.Monitors) != 1 {
		t.Fatalf("Monitors length = %d", len(result.Monitors))
	}
	if result.Monitors[0].Uptime != 0.98 {
		t.Fatalf("Uptime = %v, want 0.98", result.Monitors[0].Uptime)
	}
	if result.TimeWindowHours != 24 {
		t.Fatalf("TimeWindowHours = %d, want 24", result.TimeWindowHours)
	}
	if result.TimeWindowLabel != "24H" {
		t.Fatalf("TimeWindowLabel = %q, want 24H", result.TimeWindowLabel)
	}
}

func TestFetchGroupDataReturnsEmbedUrlWithoutFetchingUptimeKuma(t *testing.T) {
	result := fetchGroupData(context.Background(), http.DefaultClient, map[string]interface{}{
		"categoryName": "ApiPanelWatch",
		"embedUrl":     "https://status.example.com/embed/status?channelId=1",
	})

	if result.CategoryName != "ApiPanelWatch" {
		t.Fatalf("CategoryName = %q", result.CategoryName)
	}
	if result.EmbedUrl != "https://status.example.com/embed/status?channelId=1" {
		t.Fatalf("EmbedUrl = %q", result.EmbedUrl)
	}
	if result.TimeWindowHours != 24 {
		t.Fatalf("TimeWindowHours = %d, want 24", result.TimeWindowHours)
	}
	if result.TimeWindowLabel != "24H" {
		t.Fatalf("TimeWindowLabel = %q, want 24H", result.TimeWindowLabel)
	}
	if len(result.Monitors) != 0 {
		t.Fatalf("Monitors length = %d", len(result.Monitors))
	}
}
