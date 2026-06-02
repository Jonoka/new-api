package console_setting

import "testing"

func TestValidateUptimeKumaGroupsAllowsEmbedUrlWithoutKumaUrlAndSlug(t *testing.T) {
	err := validateUptimeKumaGroups(`[{"id":1,"categoryName":"ApiPanelWatch","url":"","slug":"","embedUrl":"https://status.example.com/embed/status?channelId=1"}]`)
	if err != nil {
		t.Fatalf("validateUptimeKumaGroups returned error: %v", err)
	}
}
