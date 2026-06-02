package controller

import (
	"context"
	"net/http"
	"testing"
)

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
	if len(result.Monitors) != 0 {
		t.Fatalf("Monitors length = %d", len(result.Monitors))
	}
}
