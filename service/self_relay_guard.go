package service

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func channelSelfReferenceHosts(c *gin.Context) map[string]struct{} {
	hosts := make(map[string]struct{})
	addHost := func(host string) {
		if normalized := normalizeRelayHost(host); normalized != "" {
			hosts[normalized] = struct{}{}
		}
	}
	addURLHost := func(rawURL string) {
		if parsed, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
			addHost(parsed.Host)
		}
	}

	if c != nil && c.Request != nil {
		addHost(c.Request.Host)
		for _, header := range []string{"X-Forwarded-Host", "X-Original-Host"} {
			for _, value := range strings.Split(c.GetHeader(header), ",") {
				addHost(value)
			}
		}
	}
	addURLHost(system_setting.ServerAddress)
	for _, item := range console_setting.GetApiInfo() {
		if rawURL, ok := item["url"].(string); ok {
			addURLHost(rawURL)
		}
	}
	return hosts
}

func normalizeRelayHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	if parsed, err := url.Parse(host); err == nil && parsed.Host != "" {
		host = parsed.Host
	}
	host = strings.TrimSuffix(host, ".")
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	} else if strings.HasPrefix(host, "[") && strings.Contains(host, "]") {
		host = strings.Trim(host, "[]")
	} else if colonIndex := strings.LastIndex(host, ":"); colonIndex > -1 && strings.Count(host, ":") == 1 {
		host = host[:colonIndex]
	}
	return strings.Trim(host, "[]")
}

func ChannelBaseURLMatchesLocalEndpoint(c *gin.Context, rawBaseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil || parsed.Host == "" {
		return false
	}
	channelHost := normalizeRelayHost(parsed.Host)
	if channelHost == "" {
		return false
	}
	_, found := channelSelfReferenceHosts(c)[channelHost]
	return found
}

func IsSelfReferentialChannel(c *gin.Context, channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	return ChannelBaseURLMatchesLocalEndpoint(c, channel.GetBaseURL())
}

func ValidateChannelBaseURLNotSelf(c *gin.Context, channel *model.Channel) error {
	if channel == nil || !IsSelfReferentialChannel(c, channel) {
		return nil
	}
	return fmt.Errorf("渠道上游地址不能指向当前站点或已公布的 API 域名：%s", channel.GetBaseURL())
}
