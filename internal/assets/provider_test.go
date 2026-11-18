package assets

import "testing"

func TestProviderNameOfAnUnknownKeyKeepsEveryCharacter(t *testing.T) {
	for key, want := range map[string]string{
		"cloudflare": "Cloudflare",
		"example":    "Example",
		"社内基盤":       "社内基盤",
		"":           "",
	} {
		if got := ProviderName(key); got != want {
			t.Errorf("ProviderName(%q) = %q, want %q", key, got, want)
		}
	}
}
