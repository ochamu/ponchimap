package report

import (
	"bytes"
	"cmp"
	"fmt"
	"strings"
)

type document struct {
	title    string
	preamble string
	sections []section
}

type section struct {
	heading string
	lead    string
	lines   []line
	items   []item
}

// item is the one place that describes a node, an edge, or a group. id is
// the ID of that element, and the link target of the item.
type item struct {
	id, heading string
	lines       []line
}

type line struct {
	anchor string // the ID the line is the link target of, if it is one
	body   inline
	sub    []line
}

type inline []span

// span is text, text printed as code, or a link to the element, record or
// entry ref. A link reads as its ref unless it has text.
type span struct {
	text, ref string
	code      bool
}

func text(s string) inline { return inline{{text: s}} }

func code(s string) inline { return inline{{text: s, code: true}} }

func link(id string) inline { return inline{{ref: id}} }

func linkAs(label, ref string) inline { return inline{{text: label, ref: ref}} }

// The methods copy, so that two lines can start from one inline.
func (in inline) add(more inline) inline { return append(in[:len(in):len(in)], more...) }

func (in inline) text(s string) inline { return in.add(text(s)) }

func (in inline) link(id string) inline { return in.add(link(id)) }

func joined(parts []inline, sep string) inline {
	var out inline
	for i, p := range parts {
		if i > 0 {
			out = out.text(sep)
		}
		out = out.add(p)
	}
	return out
}

func links(ids []string) inline {
	parts := make([]inline, len(ids))
	for i, id := range ids {
		parts[i] = link(id)
	}
	return joined(parts, "、")
}

func labelled(label string, body inline) line {
	return line{body: text(label + ": ").add(body)}
}

// row is a "label: value" line, and no line when there is no value.
func row(label string, value inline) []line {
	if len(value) == 0 {
		return nil
	}
	return []line{labelled(label, value)}
}

func rows(labelsAndValues ...string) []line {
	var out []line
	for i := 0; i+1 < len(labelsAndValues); i += 2 {
		if value := labelsAndValues[i+1]; value != "" {
			out = append(out, row(labelsAndValues[i], text(value))...)
		}
	}
	return out
}

func anchorTag(id string) string { return `<a id="` + id + `"></a>` }

// The anchors are written out instead of left to the headings, so that they
// do not depend on how a viewer derives heading slugs.
func (d *document) markdown() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# %s\n\n%s\n", plain(d.title), plain(d.preamble))
	for _, sec := range d.sections {
		fmt.Fprintf(&b, "\n## %s\n", plain(sec.heading))
		if sec.lead != "" {
			fmt.Fprintf(&b, "\n%s\n", plain(sec.lead))
		}
		if len(sec.lines) > 0 {
			b.WriteString("\n")
			writeLines(&b, sec.lines, "")
		}
		for _, it := range sec.items {
			fmt.Fprintf(&b, "\n%s\n\n### %s\n\n", anchorTag(it.id), plain(it.heading))
			writeLines(&b, it.lines, "")
		}
	}
	return b.Bytes()
}

func writeLines(b *bytes.Buffer, lines []line, indent string) {
	for _, l := range lines {
		b.WriteString(indent + "- ")
		if l.anchor != "" {
			b.WriteString(anchorTag(l.anchor))
		}
		for _, s := range l.body {
			switch {
			case s.ref != "":
				fmt.Fprintf(b, "[%s](#%s)", plain(cmp.Or(s.text, s.ref)), s.ref)
			case s.code:
				b.WriteString(codeSpan(s.text))
			default:
				b.WriteString(plain(s.text))
			}
		}
		b.WriteString("\n")
		writeLines(b, l.sub, indent+"  ")
	}
}

var plainReplacer = strings.NewReplacer(
	"\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "&", "\\&", "[", "\\[", "]", "\\]", "<", "\\<", ">", "\\>",
	"\r\n", " ", "\n", " ", "\r", " ",
)

// plain keeps model text from being read as Markdown or HTML structure.
func plain(s string) string { return plainReplacer.Replace(s) }

var oneLine = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

// codeSpan writes s as Markdown code, in which no sign of s is read as
// Markdown. CommonMark ends the code at a run of backticks as long as the
// one that opens it, so the fence is one longer than any run in s, and a
// space keeps a backtick at either end of s apart from the fence.
func codeSpan(s string) string {
	s = oneLine.Replace(s)
	longest, run := 0, 0
	for _, r := range s {
		if r != '`' {
			run = 0
			continue
		}
		run++
		longest = max(longest, run)
	}
	fence := strings.Repeat("`", longest+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return fence + s + fence
}
