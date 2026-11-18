// Package assets holds the fonts and the icons a figure is drawn with, and
// the display names of providers.
package assets

import (
	"unicode"
	"unicode/utf8"
)

var providers = map[string]string{
	"aws":        "AWS",
	"gcp":        "Google Cloud",
	"cloudflare": "Cloudflare",
	"firebase":   "Firebase",
	"supabase":   "Supabase",
	"vercel":     "Vercel",
	"github":     "GitHub",
}

// ProviderName returns the display name for a provider key, or the key with
// its first letter in upper case when providers does not list it.
func ProviderName(key string) string {
	if p, ok := providers[key]; ok {
		return p
	}
	first, size := utf8.DecodeRuneInString(key)
	if size == 0 {
		return ""
	}
	return string(unicode.ToUpper(first)) + key[size:]
}
