package service

import "testing"

func TestSelfUpdateAssetNamesForLinux(t *testing.T) {
	tests := []struct {
		name         string
		goarch       string
		wantAsset    string
		wantChecksum string
	}{
		{
			name:         "amd64",
			goarch:       "amd64",
			wantAsset:    "new-api-v1.0.0-rc.10",
			wantChecksum: "checksums-linux.txt",
		},
		{
			name:         "arm64",
			goarch:       "arm64",
			wantAsset:    "new-api-arm64-v1.0.0-rc.10",
			wantChecksum: "checksums-linux.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAsset, gotChecksum, err := selfUpdateAssetNames("linux", tt.goarch, "v1.0.0-rc.10")
			if err != nil {
				t.Fatalf("selfUpdateAssetNames returned error: %v", err)
			}
			if gotAsset != tt.wantAsset {
				t.Fatalf("asset = %q, want %q", gotAsset, tt.wantAsset)
			}
			if gotChecksum != tt.wantChecksum {
				t.Fatalf("checksum = %q, want %q", gotChecksum, tt.wantChecksum)
			}
		})
	}
}

func TestSelfUpdateAssetNamesRejectsUnsupportedArch(t *testing.T) {
	_, _, err := selfUpdateAssetNames("linux", "386", "v1.0.0-rc.10")
	if err == nil {
		t.Fatal("expected unsupported architecture error")
	}
}

func TestChecksumForAssetParsesSha256Manifest(t *testing.T) {
	manifest := "" +
		"56186c6e6b7493f9cc54b2297c65a363568fe6328e5d1ebe1db6b2e855a10089  new-api-v1.0.0-rc.10\n" +
		"b72df122b0f09ae9b172d6cae36bbc1ee2380aa7660c325c1a974bbb9ef1e436 *new-api-arm64-v1.0.0-rc.10\n"

	got, err := checksumForAsset(manifest, "new-api-arm64-v1.0.0-rc.10")
	if err != nil {
		t.Fatalf("checksumForAsset returned error: %v", err)
	}
	if got != "b72df122b0f09ae9b172d6cae36bbc1ee2380aa7660c325c1a974bbb9ef1e436" {
		t.Fatalf("checksum = %q", got)
	}
}

func TestValidateSelfUpdateRepo(t *testing.T) {
	if err := validateSelfUpdateRepo("moeacgx/new-api"); err != nil {
		t.Fatalf("expected repo to be valid: %v", err)
	}
	if err := validateSelfUpdateRepo("https://github.com/moeacgx/new-api"); err == nil {
		t.Fatal("expected full URL repo to be rejected")
	}
}

func TestReleaseAssetDownloadURLPrefersAPIAssetURL(t *testing.T) {
	asset := GitHubReleaseAsset{
		Name:               "new-api-v1.0.0-rc.10",
		URL:                "https://api.github.com/repos/moeacgx/new-api/releases/assets/123",
		BrowserDownloadURL: "https://github.com/moeacgx/new-api/releases/download/v1.0.0-rc.10/new-api-v1.0.0-rc.10",
	}

	if got := releaseAssetDownloadURL(asset); got != asset.URL {
		t.Fatalf("download URL = %q, want %q", got, asset.URL)
	}
}

func TestValidateGitHubDownloadURLAcceptsReleaseAssetAPIURL(t *testing.T) {
	err := validateGitHubDownloadURL("moeacgx/new-api", "https://api.github.com/repos/moeacgx/new-api/releases/assets/123")
	if err != nil {
		t.Fatalf("expected GitHub asset API URL to be valid: %v", err)
	}
}
