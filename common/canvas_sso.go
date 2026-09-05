package common

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-redis/redis/v8"
)

var CanvasSSOOrigin string
var CanvasSSOLaunchEnabled bool
var CanvasSSORDB *redis.Client

// Initialized once with the shared datastore, but without retrying single-use commands.
func InitCanvasSSORedisClient() {
	if RDB == nil || CanvasSSOOrigin == "" {
		return
	}
	options := *RDB.Options()
	options.MaxRetries = -1
	CanvasSSORDB = redis.NewClient(&options)
}

func ValidateCanvasSSOOrigin(origin string) error {
	if origin == "" {
		return nil
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		u.Opaque != "" || origin != "https://"+u.Host || strings.ToLower(u.Host) != u.Host {
		return errors.New("CANVAS_SSO_ORIGIN must be an exact canonical HTTPS origin")
	}
	portText := u.Port()
	if strings.HasSuffix(u.Host, ":") {
		return errors.New("CANVAS_SSO_ORIGIN must not contain an empty port")
	}
	if portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 || port == 443 || strconv.Itoa(port) != portText {
			return errors.New("CANVAS_SSO_ORIGIN must use a canonical non-default port from 1 to 65535")
		}
	}
	return nil
}
