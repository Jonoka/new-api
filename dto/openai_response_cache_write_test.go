package dto

import (
	"encoding/json"
	"testing"
)

func TestInputTokenDetailsAcceptsOpenAICacheWriteTokens(t *testing.T) {
	var details InputTokenDetails
	if err := json.Unmarshal([]byte(`{"cached_tokens":1920,"cache_write_tokens":2048}`), &details); err != nil {
		t.Fatal(err)
	}
	if details.CachedTokens != 1920 {
		t.Fatalf("CachedTokens = %d, want 1920", details.CachedTokens)
	}
	if details.CachedCreationTokens != 2048 {
		t.Fatalf("CachedCreationTokens = %d, want 2048", details.CachedCreationTokens)
	}
}

func TestInputTokenDetailsKeepsLegacyCachedCreationTokens(t *testing.T) {
	var details InputTokenDetails
	if err := json.Unmarshal([]byte(`{"cached_creation_tokens":300}`), &details); err != nil {
		t.Fatal(err)
	}
	if details.CachedCreationTokens != 300 {
		t.Fatalf("CachedCreationTokens = %d, want 300", details.CachedCreationTokens)
	}
}

func TestInputTokenDetailsDistinguishesMissingExplicitZeroAndNull(t *testing.T) {
	var details InputTokenDetails
	if err := json.Unmarshal([]byte(`{"cache_write_tokens":0}`), &details); err != nil {
		t.Fatal(err)
	}
	if !details.CachedCreationTokensPresent {
		t.Fatal("explicit zero should be marked present")
	}

	if err := json.Unmarshal([]byte(`{"cached_tokens":10}`), &details); err != nil {
		t.Fatal(err)
	}
	if details.CachedCreationTokensPresent {
		t.Fatal("missing field should reset presence")
	}

	if err := json.Unmarshal([]byte(`{"cache_write_tokens":null}`), &details); err != nil {
		t.Fatal(err)
	}
	if details.CachedCreationTokensPresent {
		t.Fatal("null should be treated as unavailable, not explicit zero")
	}
}
