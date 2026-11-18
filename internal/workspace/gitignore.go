package workspace

import (
	"bytes"
	"regexp"
)

// IgnoreEntry is the line of a .gitignore that keeps the workspace out of
// Git. It names the files of the folder and not the folder, so that a later
// line of the user, such as !/.ponchimap/model.json, can let one file in:
// Git does not look into a folder that is ignored as a whole.
const IgnoreEntry = "/" + Folder + "/*"

const ignoreComment = "# ponchimap: the workspace stays on this machine; it holds a report with identifiers"

// naming matches a line that names the whole workspace: the folder or all of
// its files, from the root of the .gitignore or at any depth. A negated line
// and one that is commented out match too. Each is what the user decided
// about the workspace, and a line after it would undo that.
var naming = regexp.MustCompile(`^(#[ \t]*)?!?(/|\*\*/)?` + regexp.QuoteMeta(Folder) + `(/(\*\*?)?)?$`)

// A line is read the way Git reads it: without the byte order mark at the
// start of the file, the \r before the line break, and the spaces at its
// end. A tab at the end and a space at the start are a part of the pattern
// for Git, which then names another path.
func namingLine(text []byte) int {
	text = bytes.TrimPrefix(text, []byte("\xef\xbb\xbf"))
	number := 0
	for line := range bytes.SplitSeq(text, []byte("\n")) {
		number++
		line = bytes.TrimRight(bytes.TrimSuffix(line, []byte("\r")), " ")
		if naming.Match(line) {
			return number
		}
	}
	return 0
}

func withEntry(text []byte) []byte {
	newline := "\n"
	if bytes.Contains(text, []byte("\r\n")) {
		newline = "\r\n"
	}
	out := bytes.Clone(text)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, newline...)
	}
	return append(out, ignoreComment+newline+IgnoreEntry+newline...)
}
