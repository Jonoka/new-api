package common

import "testing"

func TestValidateCanvasSSOOrigin(t *testing.T) {
	for _, origin := range []string{"", "https://canvas-2.jo2api.com", "https://canvas.test:9443"} {
		if err := ValidateCanvasSSOOrigin(origin); err != nil {
			t.Errorf("valid origin rejected: %s", origin)
		}
	}
	for _, origin := range []string{"http://canvas.test", "//canvas.test", "https://user@canvas.test", "https://canvas.test/", "https://canvas.test/path", "https://canvas.test?", "https://canvas.test?q=x", "https://canvas.test#fragment", "https://canvas.test#", "https://CANVAS.test", " https://canvas.test", "https://canvas.test\r\n"} {
		if ValidateCanvasSSOOrigin(origin) == nil {
			t.Errorf("invalid origin accepted: %q", origin)
		}
	}
}
