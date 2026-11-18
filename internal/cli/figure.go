package cli

import (
	"bytes"
	"io"
	"os"
	"slices"

	"github.com/ochamu/ponchimap/internal/diag"
)

// figureMark is in every SVG that render saves, right after the XML
// declaration. A repository can keep a figure of its own under the name that
// render saves to, so render replaces only an SVG that holds the mark and the
// PNG beside it.
var figureMark = []byte("<!-- ponchimap render wrote this file and replaces it on the next render -->")

// marked is the SVG with figureMark in it.
func marked(svg []byte) []byte {
	at := 0
	if bytes.HasPrefix(svg, []byte("<?xml")) {
		if end := bytes.Index(svg, []byte("?>")); end >= 0 {
			at = end + len("?>")
		}
	}
	return slices.Concat(svg[:at], figureMark, svg[at:])
}

// notOurs is the path of a figure file that render must not replace, or "".
// That is an SVG without figureMark, or a PNG with no SVG beside it.
func notOurs(svgPath, pngPath string) string {
	switch holds(svgPath) {
	case theirs:
		return svgPath
	case nothing:
		if holds(pngPath) != nothing {
			return pngPath
		}
	}
	return ""
}

type holding int

const (
	// nothing is no regular file. Save refuses a directory or a symbolic
	// link at a target, so only a regular file is looked into.
	nothing holding = iota
	ours
	theirs
)

// holds says whose file is at path. A file that cannot be read is taken for
// the user's.
func holds(path string) holding {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nothing
	}
	f, err := os.Open(path)
	if err != nil {
		return theirs
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	if bytes.Contains(head[:n], figureMark) {
		return ours
	}
	return theirs
}

func notReplaced(path string) diag.Diagnostic {
	return diag.Diagnostic{Severity: diag.Error, Code: "output.not_ours", Location: diag.Location{Path: path},
		Message: "ponchimap render did not write this file, so render does not replace it",
		Hint:    "Move or remove the file, or save the figure in another directory with --output."}
}
