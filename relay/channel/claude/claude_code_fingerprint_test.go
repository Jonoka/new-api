package claude

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

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
	requireClaudeCodeLegacyMetadata(t, claudeReq.Metadata)
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
	require.Equal(t, "user_0000000000000000000000000000000000000000000000000000000000000000_account__session_00000000-0000-0000-0000-000000000000", userID)
	require.Equal(t, "keep", metadata["trace"])
}

func TestConvertClaudeRequestRewritesAccountMetadataForLegacySub2APICompatibility(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	req := &dto.ClaudeRequest{
		Model:    "claude-sonnet-4-20250514",
		Metadata: []byte(`{"user_id":"user_0000000000000000000000000000000000000000000000000000000000000000_account_00000000-0000-0000-0000-000000000000_session_00000000-0000-0000-0000-000000000000","trace":"keep"}`),
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
	requireClaudeCodeLegacyMetadata(t, claudeReq.Metadata)
	var metadata map[string]interface{}
	require.NoError(t, common.Unmarshal(claudeReq.Metadata, &metadata))
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
	requireClaudeCodeLegacyMetadata(t, claudeReq.Metadata)
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
	requireClaudeCodeLegacyMetadata(t, claudeReq.Metadata)
}

func TestClaudeCodeFingerprintMatchesSub2APIMessagesClientRestriction(t *testing.T) {
	t.Parallel()

	ctx := newClaudeFingerprintTestContext()
	adaptor := &Adaptor{}
	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hi"},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-sonnet-4-20250514",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "sk-test",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	converted, err := adaptor.ConvertClaudeRequest(ctx, info, req)
	require.NoError(t, err)
	bodyBytes, err := common.Marshal(converted)
	require.NoError(t, err)
	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(bodyBytes, &body))

	headers := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(ctx, &headers, info))

	require.Regexp(t, regexp.MustCompile(`(?i)^claude-cli/\d+\.\d+\.\d+`), headers.Get("User-Agent"))
	require.NotEmpty(t, headers.Get("X-App"))
	require.NotEmpty(t, headers.Get("anthropic-beta"))
	require.NotEmpty(t, headers.Get("anthropic-version"))

	systemEntries, ok := body["system"].([]interface{})
	require.True(t, ok)
	require.NotEmpty(t, systemEntries)
	firstSystem, ok := systemEntries[0].(map[string]interface{})
	require.True(t, ok)
	require.Contains(t, firstSystem["text"], "Claude Code")

	metadata, ok := body["metadata"].(map[string]interface{})
	require.True(t, ok)
	userID, ok := metadata["user_id"].(string)
	require.True(t, ok)
	require.True(t, legacySub2APIClaudeCodeUserIDPattern.MatchString(userID))
	require.NotNil(t, parseCurrentSub2APIClaudeCodeUserID(userID))
}

func TestClaudeCodeFingerprintFinalOutboundRequestMatchesSub2APIRestrictions(t *testing.T) {
	t.Parallel()

	service.InitHttpClient()

	type capturedRequest struct {
		header http.Header
		body   []byte
		path   string
	}
	captured := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		captured <- capturedRequest{
			header: r.Header.Clone(),
			body:   body,
			path:   r.URL.Path,
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	ctx := newClaudeFingerprintTestContext()
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-sonnet-4-20250514",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeAnthropic,
			ApiType:        constant.APITypeAnthropic,
			ChannelBaseUrl: server.URL,
			ApiKey:         "sk-test",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}
	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hi"},
		},
	}

	converted, err := adaptor.ConvertClaudeRequest(ctx, info, req)
	require.NoError(t, err)
	bodyBytes, err := common.Marshal(converted)
	require.NoError(t, err)
	resp, err := adaptor.DoRequest(ctx, info, bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.(*http.Response).Body.Close()

	got := <-captured
	require.Equal(t, "/v1/messages", got.path)
	require.Regexp(t, regexp.MustCompile(`(?i)^claude-cli/\d+\.\d+\.\d+`), got.header.Get("User-Agent"))
	require.NotEmpty(t, got.header.Get("X-App"))
	require.NotEmpty(t, got.header.Get("anthropic-beta"))
	require.NotEmpty(t, got.header.Get("anthropic-version"))

	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(got.body, &body))
	systemEntries, ok := body["system"].([]interface{})
	require.True(t, ok)
	require.NotEmpty(t, systemEntries)
	firstSystem, ok := systemEntries[0].(map[string]interface{})
	require.True(t, ok)
	require.Contains(t, firstSystem["text"], "Claude Code")

	metadata, ok := body["metadata"].(map[string]interface{})
	require.True(t, ok)
	userID, ok := metadata["user_id"].(string)
	require.True(t, ok)
	require.True(t, legacySub2APIClaudeCodeUserIDPattern.MatchString(userID))
	require.NotNil(t, parseCurrentSub2APIClaudeCodeUserID(userID))
}

func TestRealClaudeCodeHeadersPassThroughWhenChannelFingerprintDisabled(t *testing.T) {
	t.Parallel()

	service.InitHttpClient()

	type capturedRequest struct {
		header http.Header
		body   []byte
	}
	captured := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		captured <- capturedRequest{
			header: r.Header.Clone(),
			body:   body,
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	ctx := newClaudeFingerprintTestContext()
	ctx.Request.Header.Set("User-Agent", "claude-cli/2.1.156 (Claude Code)")
	ctx.Request.Header.Set("X-App", "claude-code")
	ctx.Request.Header.Set("Anthropic-Beta", "claude-code-20250219")
	ctx.Request.Header.Set("Anthropic-Version", "2023-06-01")
	ctx.Request.Header.Set("Anthropic-Dangerous-Direct-Browser-Access", "true")
	ctx.Request.Header.Set("X-Stainless-Lang", "js")
	ctx.Request.Header.Set("X-Stainless-Package-Version", "0.72.0")
	ctx.Request.Header.Set("X-Stainless-OS", "Windows")
	ctx.Request.Header.Set("X-Stainless-Arch", "x64")
	ctx.Request.Header.Set("X-Stainless-Runtime", "node")
	ctx.Request.Header.Set("X-Stainless-Runtime-Version", "v24.13.0")
	ctx.Request.Header.Set("X-Stainless-Retry-Count", "0")
	ctx.Request.Header.Set("X-Stainless-Timeout", "600")
	ctx.Request.Header.Set("X-Client-Request-Id", "client-request-id")
	ctx.Request.Header.Set("X-Claude-Code-Session-Id", "session-123")

	userID := "user_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_account__session_aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-sonnet-4-6",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeAnthropic,
			ApiType:        constant.APITypeAnthropic,
			ChannelBaseUrl: server.URL,
			ApiKey:         "sk-test",
		},
	}
	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-6",
		System: []dto.ClaudeMediaMessage{
			newClaudeCodeSystemBlock(),
		},
		Metadata: []byte(`{"user_id":"` + userID + `"}`),
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hi"},
		},
	}

	converted, err := adaptor.ConvertClaudeRequest(ctx, info, req)
	require.NoError(t, err)
	bodyBytes, err := common.Marshal(converted)
	require.NoError(t, err)
	resp, err := adaptor.DoRequest(ctx, info, bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.(*http.Response).Body.Close()

	got := <-captured
	require.Equal(t, "sk-test", got.header.Get("x-api-key"))
	require.Empty(t, got.header.Get("Authorization"))
	require.Equal(t, "claude-cli/2.1.156 (Claude Code)", got.header.Get("User-Agent"))
	require.Equal(t, "claude-code", got.header.Get("X-App"))
	require.Equal(t, "claude-code-20250219", got.header.Get("Anthropic-Beta"))
	require.Equal(t, "2023-06-01", got.header.Get("Anthropic-Version"))
	require.Equal(t, "true", got.header.Get("Anthropic-Dangerous-Direct-Browser-Access"))
	require.Equal(t, "js", got.header.Get("X-Stainless-Lang"))
	require.Equal(t, "0.72.0", got.header.Get("X-Stainless-Package-Version"))
	require.Equal(t, "Windows", got.header.Get("X-Stainless-OS"))
	require.Equal(t, "x64", got.header.Get("X-Stainless-Arch"))
	require.Equal(t, "node", got.header.Get("X-Stainless-Runtime"))
	require.Equal(t, "v24.13.0", got.header.Get("X-Stainless-Runtime-Version"))
	require.Equal(t, "0", got.header.Get("X-Stainless-Retry-Count"))
	require.Equal(t, "600", got.header.Get("X-Stainless-Timeout"))
	require.Equal(t, "client-request-id", got.header.Get("X-Client-Request-Id"))
	require.Equal(t, "session-123", got.header.Get("X-Claude-Code-Session-Id"))

	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(got.body, &body))
	systemEntries, ok := body["system"].([]interface{})
	require.True(t, ok)
	require.NotEmpty(t, systemEntries)
	firstSystem, ok := systemEntries[0].(map[string]interface{})
	require.True(t, ok)
	require.Contains(t, firstSystem["text"], "Claude Code")

	metadata, ok := body["metadata"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, userID, metadata["user_id"])
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

func TestSetupRequestHeaderDoesNotPassThroughNonClaudeCodeClientHeaders(t *testing.T) {
	t.Parallel()

	ctx := newClaudeFingerprintTestContext()
	ctx.Request.Header.Set("User-Agent", "Hermes/1.0")
	ctx.Request.Header.Set("X-App", "browser")
	ctx.Request.Header.Set("X-Stainless-Lang", "python")
	ctx.Request.Header.Set("X-Client-Request-Id", "client-request-id")
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-sonnet-4-6",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ApiKey:  "sk-test",
		},
	}

	headers := http.Header{}
	err := (&Adaptor{}).SetupRequestHeader(ctx, &headers, info)
	require.NoError(t, err)

	require.Empty(t, headers.Get("User-Agent"))
	require.Empty(t, headers.Get("X-App"))
	require.Empty(t, headers.Get("X-Stainless-Lang"))
	require.Empty(t, headers.Get("X-Client-Request-Id"))
	require.Equal(t, "2023-06-01", headers.Get("anthropic-version"))
}

func TestRealClaudeCodeHeaderPassthroughKeepsModelHeaderSettings(t *testing.T) {
	claudeSettings := model_setting.GetClaudeSettings()
	originalHeaders := claudeSettings.HeadersSettings
	claudeSettings.HeadersSettings = map[string]map[string][]string{
		"claude-sonnet-4-6": {
			"anthropic-beta": {"context-1m-2025-08-07"},
		},
	}
	defer func() {
		claudeSettings.HeadersSettings = originalHeaders
	}()

	ctx := newClaudeFingerprintTestContext()
	ctx.Request.Header.Set("User-Agent", "claude-cli/2.1.156 (Claude Code)")
	ctx.Request.Header.Set("X-App", "claude-code")
	ctx.Request.Header.Set("Anthropic-Beta", "claude-code-20250219")
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-sonnet-4-6",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ApiKey:  "sk-test",
		},
	}

	headers := http.Header{}
	err := (&Adaptor{}).SetupRequestHeader(ctx, &headers, info)
	require.NoError(t, err)

	require.ElementsMatch(t,
		[]string{"claude-code-20250219", "context-1m-2025-08-07"},
		strings.Split(headers.Get("anthropic-beta"), ","),
	)
	require.Equal(t, "claude-cli/2.1.156 (Claude Code)", headers.Get("User-Agent"))
	require.Equal(t, "claude-code", headers.Get("X-App"))
}

func stringPointer(value string) *string {
	return &value
}

func requireClaudeCodeLegacyMetadata(t *testing.T, raw []byte) {
	t.Helper()

	var metadata map[string]interface{}
	require.NoError(t, common.Unmarshal(raw, &metadata))
	userID, ok := metadata["user_id"].(string)
	require.True(t, ok)
	require.Equal(t, "user_0000000000000000000000000000000000000000000000000000000000000000_account__session_00000000-0000-0000-0000-000000000000", userID)
}

var legacySub2APIClaudeCodeUserIDPattern = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[\w-]+$`)

func parseCurrentSub2APIClaudeCodeUserID(userID string) map[string]string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	legacy := regexp.MustCompile(`^user_([a-fA-F0-9]{64})_account_([a-fA-F0-9-]*)_session_([a-fA-F0-9-]{36})$`)
	if matches := legacy.FindStringSubmatch(userID); matches != nil {
		return map[string]string{
			"device_id":    matches[1],
			"account_uuid": matches[2],
			"session_id":   matches[3],
		}
	}
	var parsed struct {
		DeviceID    string `json:"device_id"`
		AccountUUID string `json:"account_uuid"`
		SessionID   string `json:"session_id"`
	}
	if err := common.Unmarshal([]byte(userID), &parsed); err != nil {
		return nil
	}
	if strings.TrimSpace(parsed.DeviceID) == "" || strings.TrimSpace(parsed.SessionID) == "" {
		return nil
	}
	return map[string]string{
		"device_id":    parsed.DeviceID,
		"account_uuid": parsed.AccountUUID,
		"session_id":   parsed.SessionID,
	}
}
