package input_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ochamu/ponchimap/internal/diag"
	"github.com/ochamu/ponchimap/internal/input"
)

// read returns the value of a YAML text as JSON, or nothing when the text
// could not be read.
func read(t *testing.T, text string) (string, diag.List) {
	t.Helper()
	doc, diags := input.DecodeYAML([]byte(text))
	if doc == nil {
		return "", diags
	}
	raw, err := json.Marshal(doc.Value)
	if err != nil {
		t.Fatalf("the value of %q is not JSON: %v", text, err)
	}
	return string(raw), diags
}

// A value is what the core schema of YAML 1.2 says it is, which is what an
// editor shows. The YAML library alone would read a date as a time, 1_000 as
// a thousand, and 0b11 as three.
func TestAYAMLValueIsReadAsTheCoreSchemaReadsIt(t *testing.T) {
	for text, want := range map[string]string{
		"a: 2026-11-15":           `{"a":"2026-11-15"}`,
		"a: 2026-11-15T10:00:00Z": `{"a":"2026-11-15T10:00:00Z"}`,
		"a: 2026-11-15 10:00:00":  `{"a":"2026-11-15 10:00:00"}`,
		"a: yes":                  `{"a":"yes"}`,
		"a: 'no'":                 `{"a":"no"}`,
		"a: on":                   `{"a":"on"}`,
		"a: 1_000":                `{"a":"1_000"}`,
		"a: 0b11":                 `{"a":"0b11"}`,
		"a: 1:30":                 `{"a":"1:30"}`,
		"a: 1.2.3":                `{"a":"1.2.3"}`,
		"a: 12 apples":            `{"a":"12 apples"}`,
		"a: true":                 `{"a":true}`,
		"a: True":                 `{"a":true}`,
		"a: FALSE":                `{"a":false}`,
		"a: tRUE":                 `{"a":"tRUE"}`,
		"a: ~":                    `{"a":null}`,
		"a: null":                 `{"a":null}`,
		"a: NULL":                 `{"a":null}`,
		"a:":                      `{"a":null}`,
		"a: nil":                  `{"a":"nil"}`,
		"a: 12":                   `{"a":12}`,
		"a: -3":                   `{"a":-3}`,
		"a: 0":                    `{"a":0}`,
		"a: 1.50":                 `{"a":1.50}`,
		"a: 1e3":                  `{"a":1e3}`,
		"a: -2.5E-3":              `{"a":-2.5E-3}`,
		"a: 12345678901234567890": `{"a":12345678901234567890}`,
		`a: "12"`:                 `{"a":"12"}`,
		"a: '017'":                `{"a":"017"}`,
		`a: "true"`:               `{"a":"true"}`,
		`a: ""`:                   `{"a":""}`,
		"a: |\n  12\n":            `{"a":"12\n"}`,
		"a: >\n  true\n":          `{"a":"true\n"}`,
		"- 1\n- x\n- [true, ~]":   `[1,"x",[true,null]]`,
		`{"a": [1, "b"]}`:         `{"a":[1,"b"]}`,
		"plain":                   `"plain"`,
		"---\na: 1\n...\n":        `{"a":1}`,
		"---\n":                   `null`,
		"\xEF\xBB\xBFa: 1\r\n":    `{"a":1}`,
		"a: &kept 1":              `{"a":1}`,
	} {
		got, diags := read(t, text)
		if got != want || len(diags) > 0 {
			t.Errorf("%q is read as %s with %v; want %s", text, got, diags, want)
		}
	}
}

// A number is read in plain decimal form only. Another form is refused, not
// converted: 017 is fifteen to one YAML reader and seventeen to the next, and
// an account ID written without quotes would lose its leading zeros.
func TestAYAMLNumberInAnotherFormIsRefused(t *testing.T) {
	for _, number := range []string{"017", "007", "-012", "+5", "0x1F", "0o17", ".5", "5.", "-.5e3", ".inf", "-.INF", "+.Inf", ".nan", ".NaN"} {
		got, diags := read(t, "a: x\nb: "+number+"\n")
		wantLines(t, diags, "2:4 input.number_form /b")
		if got == "" || len(diags) != 1 {
			continue
		}
		if d := diags[0]; strings.Contains(d.Message, number) || !strings.Contains(d.Hint, "quotes") {
			t.Errorf("%s: the diagnostic says %q with the hint %q; want it to tell of quotes and not show the number", number, d.Message, d.Hint)
		}
	}
}

func TestAYAMLNumberBeyondTheLimitsIsRefused(t *testing.T) {
	for number, read := range map[string]bool{
		strings.Repeat("9", 400): true,
		"1e400":                  true,
		strings.Repeat("9", 401): false,
		"1e401":                  false,
		"1e-401":                 false,
		"1e999999999999":         false,
	} {
		doc, diags := input.DecodeYAML([]byte("seed: " + number))
		switch {
		case read && (doc == nil || len(diags) > 0):
			t.Errorf("%.20s is refused: %v", number, diags)
		case !read:
			wantLines(t, diags, "1:7 input.number_limit /seed")
			if len(diags) == 1 && strings.Contains(diags[0].Message+diags[0].Hint, number) {
				t.Errorf("%.20s: the diagnostic shows the number", number)
			}
		}
	}
}

func TestAYAMLKeyMustBeAString(t *testing.T) {
	got, diags := read(t, `1: a
true: b
~: c
? [x, y]
: d
? {k: v}
: e
1.5: f
2026-11-15: date
"1": quoted
on: text
`)
	wantLines(t, diags,
		"1:1 input.non_string_key ",
		"2:1 input.non_string_key ",
		"3:1 input.non_string_key ",
		"4:3 input.non_string_key ",
		"6:3 input.non_string_key ",
		"8:1 input.non_string_key ")
	if want := `{"1":"quoted","2026-11-15":"date","on":"text"}`; got != want {
		t.Errorf("the value is %s; want the members with string keys, %s", got, want)
	}
	for _, d := range diags {
		if !strings.Contains(d.Hint, "quotes") {
			t.Errorf("the hint %q does not tell of quotes", d.Hint)
		}
	}
}

func TestAYAMLKeyWrittenTwiceIsReportedAtTheSecond(t *testing.T) {
	got, diags := read(t, "a: 1\nb:\n  k: x\n  \"k\": y\n'a': 2\n")
	wantLines(t, diags, "4:3 input.duplicate_key /b/k", "5:1 input.duplicate_key /a")
	if want := `{"a":2,"b":{"k":"y"}}`; got != want {
		t.Errorf("the value is %s, want %s", got, want)
	}
}

// Aliases, tags, and merge keys are not read. An alias is never expanded, so
// a few lines of aliases to aliases cannot grow into millions of values.
func TestYAMLAliasesTagsAndMergeKeysAreNotRead(t *testing.T) {
	_, diags := read(t, `base: &b [n1]
use: *b
text: !!str 12
custom: !mine {a: 1}
group:
  <<: {id: g}
  label: x
? *b
: y
`)
	wantLines(t, diags,
		"2:6 input.unsupported /use",
		"3:7 input.unsupported /text",
		"4:9 input.unsupported /custom",
		"6:3 input.unsupported /group",
		"8:3 input.unsupported ")
	for i, word := range []string{"alias", "tag", "tag", "merge key", "alias"} {
		if i < len(diags) && (!strings.Contains(diags[i].Message, word) || diags[i].Hint == "") {
			t.Errorf("diagnostic %d says %q with the hint %q; want it to name the %s and say what to write", i+1, diags[i].Message, diags[i].Hint, word)
		}
	}

	// The tag "!" alone makes a scalar a string. The YAML library reads the
	// scalar without marking the tag, so it is found in the text.
	got, diags := read(t, "a: ! 5\nb: &anchor ! true\n! c: 1\n題: {日本語: ! 5, d: [1, ! 2]}\ne: \"!not a tag\"\nf: &kept 5\n")
	wantLines(t, diags,
		"1:4 input.unsupported /a",
		"2:4 input.unsupported /b",
		"3:1 input.unsupported ",
		"4:10 input.unsupported /題/日本語",
		"4:22 input.unsupported /題/d/1")
	if got != `{"a":null,"b":null,"e":"!not a tag","f":5,"題":{"d":[1,null],"日本語":null}}` {
		t.Errorf("the rest is read as %s", got)
	}

	// Lines and comments may stand between an anchor and the tag.
	for text, want := range map[string]string{
		"a: &x\n  ! 5\n":                         "1:4 input.unsupported /a",
		"a: &x # a comment\n\n  # more\n  ! 5\n": "1:4 input.unsupported /a",
		"a: &x\n  !!int 5\n":                     "1:4 input.unsupported /a",
		"a:\n  ! 5\n":                            "2:3 input.unsupported /a",
	} {
		_, diags := read(t, text)
		wantLines(t, diags, want)
	}
	// An anchor without a tag is read, with a value or without one.
	if got, diags := read(t, "a: &x\nb: &y 1\nc: [&z , 2]\nd: &w\n  # a comment\n  3\n"); len(diags) > 0 || got != `{"a":null,"b":1,"c":[null,2],"d":3}` {
		t.Errorf("anchors without tags are read as %s with %v", got, diags)
	}

	var laughs strings.Builder
	laughs.WriteString("a0: &a0 [x, x, x, x, x, x, x, x, x, x]\n")
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&laughs, "a%d: &a%d [%s]\n", i, i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*a%d, ", i-1), 10), ", "))
	}
	laughs.WriteString("self: &s [*s]\n")
	_, diags = read(t, laughs.String())
	if len(diags) != 301 {
		t.Errorf("%d diagnostics; want one for each of the 301 aliases", len(diags))
	}
}

// A directive line is not read, and the diagnostic says which line it is. The
// YAML library alone refuses the line "%YAML 1.2" without a place.
func TestAYAMLDirectiveIsRefusedWhereItStands(t *testing.T) {
	for text, want := range map[string]string{
		"%YAML 1.2\n---\na: 1\n": "1:1 input.unsupported ",
		"%YAML 1.1\n---\na: 1\n": "1:1 input.unsupported ",
		"# a comment\n\n  # another\n%TAG !e! tag:e,2000:\n---\na: !e!x 1\n": "4:1 input.unsupported ",
		"\r\n# a comment\r\n%YAML 1.2\r\n---\r\na: 1\r\n":                    "3:1 input.unsupported ",
	} {
		got, diags := read(t, text)
		wantLines(t, diags, want)
		if got != "" {
			t.Errorf("%q: a value is returned: %s", text, got)
		}
		if len(diags) == 1 && (!strings.Contains(diags[0].Message, "directive") || diags[0].Hint == "") {
			t.Errorf("%q: the diagnostic says %q with the hint %q", text, diags[0].Message, diags[0].Hint)
		}
	}
	for _, text := range []string{"--- # the document starts\na: 1\n", "a: \"100%\"\n", "# %YAML 1.2\na: 1\n"} {
		if _, diags := read(t, text); len(diags) > 0 {
			t.Errorf("%q: %v", text, diags)
		}
	}
}

func TestAYAMLFileHoldsOneDocument(t *testing.T) {
	for text, want := range map[string]string{
		"":                           "1:1 input.syntax ",
		"   \n":                      "2:1 input.syntax ",
		"# only a comment\n":         "2:1 input.syntax ",
		"a: 1\n---\nb: 2\n":          "2:1 input.trailing_data ",
		"a: 1\n---\n":                "2:1 input.trailing_data ",
		"a: 1\n...\n---\nb: 2\n":     "3:1 input.trailing_data ",
		"a: 1\n---\nb: 2\n---\nc: 3": "2:1 input.trailing_data ",
	} {
		got, diags := read(t, text)
		wantLines(t, diags, want)
		if strings.Contains(want, "trailing_data") && got != `{"a":1}` {
			t.Errorf("%q: the first document is read as %s", text, got)
		}
		if len(diags) == 1 && diags[0].Hint == "" {
			t.Errorf("%q: the diagnostic has no hint", text)
		}
	}
}

func TestAYAMLTextMustBeUTF8WithoutControlCharacters(t *testing.T) {
	for text, want := range map[string]string{
		"a: 1\nb: \x83\x56\n":       "2:4 input.invalid_utf8 ",
		"\xFF\xFEa\x00:\x00":        "1:1 input.invalid_utf8 ",
		"a: 1\nb: \"x\x1b[2Ky\"\n":  "2:6 input.syntax ",
		"名前: あ\x00\n":               "1:6 input.syntax ",
		"a: 1\r\nb: 2\r\nc: \x7f\n": "3:4 input.syntax ",
	} {
		_, diags := read(t, text)
		wantLines(t, diags, want)
		if len(diags) == 1 && diags[0].Hint == "" {
			t.Errorf("%q: the diagnostic has no hint", text)
		}
	}
}

// The YAML library says what a syntax error is in a fixed text, with a line
// and no column, and its text can quote the input. For the errors that a
// person who writes YAML by hand meets, the tool says in its own words what
// is wrong and where to look, and shows nothing of the input.
//
// The line is the one the library means: where the colon, the character, or
// the hyphen stands, or where the brackets, the string, the list, or the
// mapping with the problem starts. When that start is the first line of the
// file, the library names the line at which it stopped reading.
func TestAYAMLSyntaxErrorSaysWhatIsWrongAndWhereToLook(t *testing.T) {
	for name, c := range map[string]struct {
		text string
		line int
		says string
	}{
		"a colon without a space, seen at the colon of the next line":  {"a: 1\nb:\n  c:secret-token\n  d: 2\n", 4, "colon followed by a space"},
		"a hyphen without a space, seen at the colon of the next line": {"a:\n  -b: secret-token\n   c: 2\n", 3, "colon followed by a space"},
		"a colon in a plain value":                                     {"a: 1\nb: secret-token: x\n", 2, "colon followed by a space"},
		"a colon in a plain value on the first line":                   {"a: b: secret-token\n", 1, "colon followed by a space"},
		"a tab for indentation":                                        {"a:\n  b: 1\nc:\n\td: secret-token\n", 4, "tab"},
		"a tab on the line after a value":                              {"a:\n  b: secret-token\n\tc: 1\n", 2, "a tab indents a line"},
		"a value that starts with a reserved character":                {"a: 1\nb: @secret-token\n", 2, "reserves"},
		"a key indented unlike the others":                             {"a:\n  b: 1\n c: secret-token\n", 3, "does not fit the mapping"},
		"a line under a list that is not an item":                      {"a:\n  - b: 1\n    c: 2\n  d: secret-token\n", 2, "does not fit the list"},
		"a bracket that is not shut":                                   {"a: 1\nb: 2\nc: [secret-token, x\nd: 3\n", 3, "brackets"},
		"a brace that is not shut":                                     {"a: 1\nb: {c: secret-token\nd: 2\n", 2, "braces"},
		"a quote that is not shut":                                     {"a: 1\nb: \"secret-token\nc: 2\n", 2, "quoted string is not closed"},
		"a quote that is not shut on the first line":                   {"a: \"secret-token\nb: 2\n", 3, "quoted string is not closed"},
		"an escape that YAML does not know":                            {"a: 1\nb: \"C:\\secret-token\"\n", 2, "escape"},
		"a key without a space after its colon":                        {"a:\n  b: 1\n  c:secret-token\n", 3, "not followed by a colon and a space"},
		"a list item after a key on the same line":                     {"a: 1\nb: - secret-token\n", 2, "list item"},
		"thousands of levels":                                          {"a: " + strings.Repeat("[", 10001), 1, "thousands of levels"},
		"a problem the tool has no words for":                          {"a: 1\nb: |secret-token\n  t\n", 2, "near this line"},
	} {
		_, diags := read(t, c.text)
		if len(diags) != 1 || diags[0].Code != "input.syntax" {
			t.Errorf("%s: got %v, want one input.syntax", name, diags)
			continue
		}
		d := diags[0]
		if d.Location.Line != c.line || d.Location.Column != 0 || !strings.Contains(d.Message, c.says) {
			t.Errorf("%s: %d:%d %q; want line %d, no column, and a message with %q", name, d.Location.Line, d.Location.Column, d.Message, c.line, c.says)
		}
		if said := d.String() + d.Hint; strings.Contains(said, "secret-token") || d.Hint == "" {
			t.Errorf("%s: the diagnostic says %q", name, said)
		}
		if want := fmt.Sprintf("file.yaml:%d: error input.syntax:", c.line); !strings.HasPrefix(withPath(d, "file.yaml").String(), want) {
			t.Errorf("%s: the line for a person is %q; want it to start with %q", name, withPath(d, "file.yaml"), want)
		}
	}

	// The colon of the next line is where a missing space shows, so the hint
	// sends the reader to the line before as well.
	_, diags := read(t, "a: 1\nb:\n  c:x\n  d: 2\n")
	if len(diags) != 1 || !strings.Contains(diags[0].Hint, "line before") {
		t.Errorf("the hint for a colon without a space is %v; want it to name the line before", diags)
	}

	// An alias without an anchor is the one error for which the library
	// names no line.
	_, diags = read(t, "a: 1\nb: *secret-token\n")
	if len(diags) != 1 || diags[0].Code != "input.syntax" || diags[0].Location.Line != 0 || !strings.Contains(diags[0].Message, "anchor") {
		t.Fatalf("an alias without an anchor: got %v, want one input.syntax about the anchor, without a line", diags)
	}
	if said := diags[0].String() + diags[0].Hint; strings.Contains(said, "secret-token") || diags[0].Hint == "" {
		t.Errorf("an alias without an anchor: the diagnostic says %q", said)
	}
}

func withPath(d diag.Diagnostic, path string) diag.Diagnostic {
	d.Location.Path = path
	return d
}

// Objects and arrays are read 64 levels deep in YAML as in JSON.
func TestYAMLNestingIsReadSixtyFourLevelsDeep(t *testing.T) {
	nested := func(levels int) string {
		return "a:\n  b: " + strings.Repeat("[", levels-2) + strings.Repeat("]", levels-2) + "\n"
	}
	if got, diags := read(t, nested(64)); got == "" || len(diags) > 0 {
		t.Errorf("64 levels are refused: %v", diags)
	}
	for _, levels := range []int{65, 5000} {
		got, diags := read(t, nested(levels))
		wantLines(t, diags, "2:68 input.depth_limit /a/b"+strings.Repeat("/0", 62))
		if got != "" {
			t.Errorf("%d levels: a value was returned", levels)
		}
	}
	if _, diags := read(t, strings.Repeat("- ", 20000)+"x"); len(diags) != 1 || diags[0].Code != "input.syntax" {
		t.Errorf("twenty thousand levels give %v; want the one syntax error of the YAML reader", diags)
	}
}

// Every diagnostic carries the JSON Pointer of its value. The pointer of a
// value takes 1024 bytes at most, so the pointers of many diagnostics cannot
// take thousands of times the text: without the limit, a text of 264 KB with
// 40000 refused values under 60 KB of keys took 3.9 GB.
func TestTheKeysThatLeadToAYAMLValueTake1024BytesAtMost(t *testing.T) {
	key := func(length int) string { return strings.Repeat("k", length) }
	if got, diags := read(t, key(1023)+": 1\n"); got == "" || len(diags) > 0 {
		t.Errorf("a pointer of 1024 bytes is refused: %.200v", diags)
	}
	got, diags := read(t, "a: 1\n"+key(1024)+": 1\n")
	wantLines(t, diags, "2:1 input.pointer_limit ")
	if got != "" || !strings.Contains(diags[0].Message+diags[0].Hint, "1024") {
		t.Errorf("a pointer of 1025 bytes: value %q, diagnostic %.200q; want no value and the limit of 1024", got, diags[0].Message+diags[0].Hint)
	}

	// Ten keys of 100 bytes lead to a pointer of 1010 bytes. An eleventh
	// does not fit, and neither does an item under a key of 1022 bytes.
	var nested strings.Builder
	for i := range 11 {
		nested.WriteString(strings.Repeat(" ", i) + key(100) + ":\n")
	}
	_, diags = read(t, nested.String()+strings.Repeat(" ", 11)+"x\n")
	wantLines(t, diags, "11:11 input.pointer_limit "+strings.Repeat("/"+key(100), 10))
	_, diags = read(t, key(1022)+":\n  - 017\n  - 017\n")
	wantLines(t, diags, "2:5 input.pointer_limit /"+key(1022))

	// Thousands of refused values under long keys give the one diagnostic.
	_, diags = read(t, key(1000)+":\n  "+key(1000)+": ["+strings.Repeat("017, ", 5000)+"017]\n")
	if len(diags) != 1 || diags[0].Code != "input.pointer_limit" {
		t.Errorf("%d diagnostics; want the one about the limit", len(diags))
	}
}

func TestAYAMLColumnCountsCharacters(t *testing.T) {
	_, diags := read(t, "題名: \"構成図\"\nlabels: {注文: \"受付\", 在庫: 017}\n")
	wantLines(t, diags, "2:24 input.number_form /labels/在庫")
}
