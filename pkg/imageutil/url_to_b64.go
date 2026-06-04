package imageutil

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const DefaultMaxImageBytes int64 = 20 << 20

// ConvertImageURLResponseToB64 rewrites OpenAI-compatible image response
// data[].url entries into data[].b64_json entries. If the response already
// contains b64_json, it is returned unchanged. It does not write to disk.
func ConvertImageURLResponseToB64(ctx context.Context, body []byte) ([]byte, bool, error) {
	return ConvertImageURLResponseToB64WithClient(ctx, body, &http.Client{Timeout: 20 * time.Second}, DefaultMaxImageBytes)
}

func ConvertImageURLResponseToB64WithClient(ctx context.Context, body []byte, client *http.Client, maxBytes int64) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, false, err
	}
	data, _ := payload["data"].([]any)
	if len(data) == 0 {
		return body, false, nil
	}
	for _, item := range data {
		m, _ := item.(map[string]any)
		if b64, _ := m["b64_json"].(string); strings.TrimSpace(b64) != "" {
			return body, false, nil
		}
	}
	changed := false
	for _, item := range data {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		rawURL, _ := m["url"].(string)
		if strings.TrimSpace(rawURL) == "" {
			continue
		}
		b64, err := DownloadImageAsB64(ctx, client, rawURL, maxBytes)
		if err != nil {
			return nil, false, err
		}
		m["b64_json"] = b64
		delete(m, "url")
		changed = true
	}
	if !changed {
		return body, false, nil
	}
	out, err := common.Marshal(payload)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func DownloadImageAsB64(ctx context.Context, client *http.Client, rawURL string, maxBytes int64) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxImageBytes
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported image url scheme: %s", u.Scheme)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download image failed: status %d", resp.StatusCode)
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if ct != "" && !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "application/octet-stream") {
		return "", fmt.Errorf("download image returned non-image content-type: %s", ct)
	}
	limited := io.LimitReader(resp.Body, maxBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	if int64(len(buf)) > maxBytes {
		return "", fmt.Errorf("download image exceeds max size %d bytes", maxBytes)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
