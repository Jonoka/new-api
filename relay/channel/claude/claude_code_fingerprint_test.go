package claude

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newClaudeFingerprintTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx
}

func TestConvertClaudeRequestKeepsBodyUnchangedByDefault(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	req := &dto.ClaudeRequest{
		Model:    "claude-sonnet-4-20250514",
		System:   "original system",
		Metadata: []byte(`{"user_id":"origin-user"}`),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	converted, err := adaptor.ConvertClaudeRequest(newClaudeFingerprintTestContext(), info, req)
	require.NoError(t, err)

	claudeReq, ok := converted.(*dto.ClaudeRequest)
	require.True(t, ok)
	require.Equal(t, "original system", claudeReq.System)
	require.JSONEq(t, `{"user_id":"origin-user"}`, string(claudeReq.Metadata))
}

func TestConvertClaudeRequestAddsClaudeCodeSystemAndMetadata(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-20250514",
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	converted, err := adaptor.ConvertClaudeRequest(newClaudeFingerprintTestContext(), info, req)
	require.NoError(t, err)

	claudeReq := converted.(*dto.ClaudeRequest)
	system := claudeReq.ParseSystem()
	require.Len(t, system, 1)
	require.Equal(t, "text", system[0].Type)
	require.Contains(t, system[0].GetText(), "Claude Code")
	require.JSONEq(t, `{"user_id":"user_0000000000000000000000000000000000000000000000000000000000000000_account_00000000-0000-0000-0000-000000000000_session_00000000-0000-0000-0000-000000000000"}`, string(claudeReq.Metadata))
}

func TestConvertClaudeRequestReplacesEmptyStringSystemWithClaudeCodeSystem(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	req := &dto.ClaudeRequest{
		Model:  "claude-sonnet-4-20250514",
		System: "",
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	converted, err := adaptor.ConvertClaudeRequest(newClaudeFingerprintTestContext(), info, req)
	require.NoError(t, err)

	claudeReq := converted.(*dto.ClaudeRequest)
	system := claudeReq.ParseSystem()
	require.Len(t, system, 1)
	require.Contains(t, system[0].GetText(), "Claude Code")
}

func TestConvertClaudeRequestPrependsClaudeCodeSystemToStringSystem(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	req := &dto.ClaudeRequest{
		Model:  "claude-sonnet-4-20250514",
		System: "existing string system",
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	converted, err := adaptor.ConvertClaudeRequest(newClaudeFingerprintTestContext(), info, req)
	require.NoError(t, err)

	claudeReq := converted.(*dto.ClaudeRequest)
	system := claudeReq.ParseSystem()
	require.Len(t, system, 2)
	require.Contains(t, system[0].GetText(), "Claude Code")
	require.Equal(t, "existing string system", system[1].GetText())
}

func TestConvertClaudeRequestPrependsClaudeCodeSystemWithoutOverwritingMetadataUserId(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-20250514",
		System: []dto.ClaudeMediaMessage{
			{Type: "text", Text: stringPointer("existing system")},
		},
		Metadata: []byte(`{"user_id":"real-user","trace":"keep"}`),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	converted, err := adaptor.ConvertClaudeRequest(newClaudeFingerprintTestContext(), info, req)
	require.NoError(t, err)

	claudeReq := converted.(*dto.ClaudeRequest)
	system := claudeReq.ParseSystem()
	require.Len(t, system, 2)
	require.Contains(t, system[0].GetText(), "Claude Code")
	require.Equal(t, "existing system", system[1].GetText())
	require.JSONEq(t, `{"user_id":"real-user","trace":"keep"}`, string(claudeReq.Metadata))
}

func TestSetupRequestHeaderAddsClaudeCodeFingerprintHeaders(t *testing.T) {
	t.Parallel()

	ctx := newClaudeFingerprintTestContext()
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-sonnet-4-20250514",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "sk-test",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	headers := http.Header{}
	err := (&Adaptor{}).SetupRequestHeader(ctx, &headers, info)
	require.NoError(t, err)

	require.Equal(t, "sk-test", headers.Get("x-api-key"))
	require.Equal(t, "claude-cli/2.1.114 (external, sdk-cli)", headers.Get("User-Agent"))
	require.Equal(t, "cli", headers.Get("X-App"))
	require.Equal(t, "2023-06-01", headers.Get("anthropic-version"))
	require.Equal(t, "true", headers.Get("anthropic-dangerous-direct-browser-access"))
	require.Contains(t, headers.Get("anthropic-beta"), "claude-code-20250219")
	require.Contains(t, headers.Get("anthropic-beta"), "advisor-tool-2026-03-01")
}

func stringPointer(value string) *string {
	return &value
}
