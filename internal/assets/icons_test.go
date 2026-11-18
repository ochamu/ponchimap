package assets

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/schema"
)

const (
	svgNamespace = "http://www.w3.org/2000/svg"
	maxIconBytes = 1536
)

var allowedIconElements = map[string]bool{
	"svg": true, "g": true, "rect": true, "circle": true, "ellipse": true,
	"line": true, "polyline": true, "polygon": true, "path": true,
}

// checkIconSVG reports the first rule of icons/README.md that raw breaks.
func checkIconSVG(raw []byte) error {
	if len(raw) > maxIconBytes {
		return fmt.Errorf("%d bytes, limit is %d", len(raw), maxIconBytes)
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	depth := 0
	sawRoot := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("not well-formed XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != svgNamespace {
				return fmt.Errorf("<%s> is not in the SVG namespace", t.Name.Local)
			}
			if !allowedIconElements[t.Name.Local] {
				return fmt.Errorf("<%s> is not an allowed element", t.Name.Local)
			}
			if depth == 0 {
				if t.Name.Local != "svg" {
					return fmt.Errorf("root element is <%s>, want <svg>", t.Name.Local)
				}
				sawRoot = true
				if got := attr(t, "viewBox"); got != "0 0 48 48" {
					return fmt.Errorf("viewBox is %q, want \"0 0 48 48\"", got)
				}
			}
			for _, a := range t.Attr {
				if a.Name.Local == "href" {
					return fmt.Errorf("<%s> has an href reference", t.Name.Local)
				}
				if strings.Contains(a.Value, "url(") {
					return fmt.Errorf("<%s> %s uses a url() reference", t.Name.Local, a.Name.Local)
				}
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return errors.New("contains text content")
			}
		}
	}
	if !sawRoot {
		return errors.New("no <svg> root element")
	}
	return nil
}

func attr(el xml.StartElement, name string) string {
	for _, a := range el.Attr {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func embeddedIconKeys(t *testing.T) []string {
	t.Helper()
	entries, err := iconFS.ReadDir("icons")
	if err != nil {
		t.Fatalf("read embedded icons: %v", err)
	}
	var keys []string
	for _, e := range entries {
		keys = append(keys, strings.TrimSuffix(e.Name(), ".svg"))
	}
	return keys
}

// A findings file names a category through the schema, and the card draws the
// icon of that name, so the two lists must agree.
func TestTheSchemaAllowsEveryIconButTheGenericOneAsACategory(t *testing.T) {
	var s struct {
		Defs struct {
			Node struct {
				Properties struct {
					Attributes struct {
						Properties struct {
							Category struct {
								Enum []string `json:"enum"`
							} `json:"category"`
						} `json:"properties"`
					} `json:"attributes"`
				} `json:"properties"`
			} `json:"node"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schema.Model, &s); err != nil {
		t.Fatal(err)
	}
	allowed := slices.Sorted(slices.Values(s.Defs.Node.Properties.Attributes.Properties.Category.Enum))
	icons := slices.DeleteFunc(embeddedIconKeys(t), func(key string) bool { return key == GenericIcon })
	slices.Sort(icons)
	if !slices.Equal(allowed, icons) {
		t.Errorf("attributes.category in model.schema.json allows %q, want the embedded icons but generic, %q", allowed, icons)
	}
}

func TestIconsAreDistinct(t *testing.T) {
	set, seen := Bundled(), map[string]string{}
	for _, key := range embeddedIconKeys(t) {
		raw := string(set.Icon(key))
		if other, dup := seen[raw]; dup {
			t.Errorf("icons %q and %q are identical", other, key)
		}
		seen[raw] = key
	}
}

func TestUnknownKeyFallsBackToGeneric(t *testing.T) {
	set := Bundled()
	generic := set.Icon(GenericIcon)
	if err := checkIconSVG(generic); err != nil {
		t.Fatalf("generic icon: %v", err)
	}
	for _, key := range []string{"no-such-category", "", ".", "..", "../catalog", "compute/../compute", "compute.svg", "a\x00b"} {
		if got := set.Icon(key); !bytes.Equal(got, generic) {
			t.Errorf("Icon(%q) did not return the generic icon", key)
		}
	}
}

func TestEmbeddedIconsMeetRendererConstraints(t *testing.T) {
	set, keys := Bundled(), embeddedIconKeys(t)
	if len(keys) == 0 {
		t.Fatal("no icons are embedded")
	}
	for _, key := range keys {
		if err := checkIconSVG(set.Icon(key)); err != nil {
			t.Errorf("icons/%s.svg: %v", key, err)
		}
	}
}

func TestIconCheckRejectsForbiddenConstructs(t *testing.T) {
	wrap := func(body string) string {
		return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48">` + body + `</svg>`
	}
	if err := checkIconSVG([]byte(wrap(`<g fill="none"><path d="M1 1h2"/></g>`))); err != nil {
		t.Fatalf("a conforming icon was rejected: %v", err)
	}
	bad := map[string]string{
		"malformed XML":    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48"><rect></svg>`,
		"missing xmlns":    `<svg viewBox="0 0 48 48"><rect width="1" height="1"/></svg>`,
		"wrong viewBox":    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"/>`,
		"script":           wrap(`<script>alert(1)</script>`),
		"style block":      wrap(`<style>rect{fill:red}</style>`),
		"text":             wrap(`<text x="1" y="9">A</text>`),
		"stray characters": wrap(`A`),
		"use":              wrap(`<use href="#a"/>`),
		"gradient":         wrap(`<linearGradient id="g"/>`),
		"filter":           wrap(`<filter id="f"/>`),
		"image":            wrap(`<image href="https://example.com/a.png"/>`),
		"xlink href":       `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 48 48"><path xlink:href="#a"/></svg>`,
		"url reference":    wrap(`<rect width="1" height="1" fill="url(#g)"/>`),
		"too large":        wrap(`<path d="` + strings.Repeat("M1 1h2", 300) + `"/>`),
	}
	for name, svg := range bad {
		if checkIconSVG([]byte(svg)) == nil {
			t.Errorf("%s: accepted, want rejection", name)
		}
	}
}
