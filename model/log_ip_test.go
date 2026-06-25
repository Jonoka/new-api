package model

import (
	"strings"
	"testing"
)

func TestShouldRecordLogIp(t *testing.T) {
	tests := []struct {
		name               string
		adminForceRecordIp bool
		userRecordIpLog    bool
		wantShouldRecordIp bool
	}{
		{
			name:               "disabled globally and by user",
			adminForceRecordIp: false,
			userRecordIpLog:    false,
			wantShouldRecordIp: false,
		},
		{
			name:               "enabled by user",
			adminForceRecordIp: false,
			userRecordIpLog:    true,
			wantShouldRecordIp: true,
		},
		{
			name:               "forced globally overrides user disabled",
			adminForceRecordIp: true,
			userRecordIpLog:    false,
			wantShouldRecordIp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRecordLogIp(tt.adminForceRecordIp, tt.userRecordIpLog); got != tt.wantShouldRecordIp {
				t.Fatalf("shouldRecordLogIp() = %v, want %v", got, tt.wantShouldRecordIp)
			}
		})
	}
}

func TestFormatUserLogsHidesIp(t *testing.T) {
	logs := []*Log{
		{
			Id:    99,
			Ip:    "203.0.113.10",
			Other: `{"admin_info":{"server_ip":"10.0.0.1"},"visible":"yes"}`,
		},
	}

	formatUserLogs(logs, 0)

	if logs[0].Ip != "" {
		t.Fatalf("formatUserLogs() exposed ip = %q, want empty", logs[0].Ip)
	}
	if strings.Contains(logs[0].Other, "admin_info") {
		t.Fatalf("formatUserLogs() exposed admin_info in Other = %s", logs[0].Other)
	}
}
