package common

import (
	"errors"
	"net/url"
	"strings"
)

var CanvasSSOOrigin string
var CanvasSSOLaunchEnabled bool

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
	return nil
}
