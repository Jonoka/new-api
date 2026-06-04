package claude

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
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

func TestConvertClaudeRequestNormalizesExistingClaudeCodeStringSystem(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	req := &dto.ClaudeRequest{
		Model:  "claude-sonnet-4-20250514",
		System: "You are Claude Code, Anthropic's official CLI for Claude.",
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
	require.Equal(t, "You are Claude Code, Anthropic's official CLI for Claude.", system[0].GetText())
}

func TestConvertClaudeRequestRewritesInvalidMetadataUserID(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	req := &dto.ClaudeRequest{
		Model:    "claude-sonnet-4-20250514",
		Metadata: []byte(`{"user_id":"hermes-user","trace":"keep"}`),
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
	var metadata map[string]interface{}
	require.NoError(t, common.Unmarshal(claudeReq.Metadata, &metadata))
	userID, ok := metadata["user_id"].(string)
	require.True(t, ok)
	require.Regexp(t, regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account_[a-fA-F0-9-]*_session_[a-fA-F0-9-]{36}$`), userID)
	require.Equal(t, "keep", metadata["trace"])
}

func TestConvertClaudeRequestAddsClaudeCodeFingerprintWhenTransportFingerprintEnabled(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-20250514",
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeTransportFingerprintEnabled: true,
			},
		},
	}

	converted, err := adaptor.ConvertClaudeRequest(newClaudeFingerprintTestContext(), info, req)
	require.NoError(t, err)

	claudeReq := converted.(*dto.ClaudeRequest)
	system := claudeReq.ParseSystem()
	require.Len(t, system, 1)
	require.Contains(t, system[0].GetText(), "Claude Code")
	require.JSONEq(t, `{"user_id":"user_0000000000000000000000000000000000000000000000000000000000000000_account_00000000-0000-0000-0000-000000000000_session_00000000-0000-0000-0000-000000000000"}`, string(claudeReq.Metadata))
}

func TestConvertOpenAIRequestAddsClaudeCodeFingerprint(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	req := &dto.GeneralOpenAIRequest{
		Model: "claude-haiku-4-5-20251001",
		Messages: []dto.Message{
			{Role: "user", Content: "hi"},
		},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	converted, err := adaptor.ConvertOpenAIRequest(newClaudeFingerprintTestContext(), info, req)
	require.NoError(t, err)

	claudeReq := converted.(*dto.ClaudeRequest)
	system := claudeReq.ParseSystem()
	require.Len(t, system, 1)
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

	existingClaudeCodeUserID := "user_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_account__session_aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	adaptor := &Adaptor{}
	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-20250514",
		System: []dto.ClaudeMediaMessage{
			{Type: "text", Text: stringPointer("existing system")},
		},
		Metadata: []byte(`{"user_id":"` + existingClaudeCodeUserID + `","trace":"keep"}`),
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
	require.JSONEq(t, `{"user_id":"`+existingClaudeCodeUserID+`","trace":"keep"}`, string(claudeReq.Metadata))
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
	require.Equal(t, "Bearer sk-test", headers.Get("Authorization"))
	require.Equal(t, "claude-cli/2.1.92 (external, cli)", headers.Get("User-Agent"))
	require.Equal(t, "cli", headers.Get("X-App"))
	require.Equal(t, "2023-06-01", headers.Get("anthropic-version"))
	require.Equal(t, "true", headers.Get("anthropic-dangerous-direct-browser-access"))
	require.Equal(t, "js", headers.Get("X-Stainless-Lang"))
	require.Equal(t, "0.70.0", headers.Get("X-Stainless-Package-Version"))
	require.Equal(t, "Linux", headers.Get("X-Stainless-OS"))
	require.Equal(t, "arm64", headers.Get("X-Stainless-Arch"))
	require.Equal(t, "node", headers.Get("X-Stainless-Runtime"))
	require.Equal(t, "v24.13.0", headers.Get("X-Stainless-Runtime-Version"))
	require.Equal(t, "0", headers.Get("X-Stainless-Retry-Count"))
	require.Equal(t, "600", headers.Get("X-Stainless-Timeout"))
	require.NotEmpty(t, headers.Get("x-client-request-id"))
	require.Contains(t, headers.Get("anthropic-beta"), "claude-code-20250219")
	require.Contains(t, headers.Get("anthropic-beta"), "oauth-2025-04-20")
	require.Contains(t, headers.Get("anthropic-beta"), "extended-cache-ttl-2025-04-11")
}

func TestSetupRequestHeaderAddsClaudeCodeFingerprintHeadersWhenTransportFingerprintEnabled(t *testing.T) {
	t.Parallel()

	ctx := newClaudeFingerprintTestContext()
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-sonnet-4-20250514",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "sk-test",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeTransportFingerprintEnabled: true,
			},
		},
	}

	headers := http.Header{}
	err := (&Adaptor{}).SetupRequestHeader(ctx, &headers, info)
	require.NoError(t, err)

	require.Equal(t, "sk-test", headers.Get("x-api-key"))
	require.Equal(t, "Bearer sk-test", headers.Get("Authorization"))
	require.Equal(t, "claude-cli/2.1.92 (external, cli)", headers.Get("User-Agent"))
	require.Equal(t, "cli", headers.Get("X-App"))
	require.Equal(t, "2023-06-01", headers.Get("anthropic-version"))
	require.Equal(t, "true", headers.Get("anthropic-dangerous-direct-browser-access"))
	require.Contains(t, headers.Get("anthropic-beta"), "claude-code-20250219")
	require.Contains(t, headers.Get("anthropic-beta"), "oauth-2025-04-20")
	require.Contains(t, headers.Get("anthropic-beta"), "extended-cache-ttl-2025-04-11")
}

func stringPointer(value string) *string {
	return &value
}
