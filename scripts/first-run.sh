#!/bin/sh
# Runs the sample route of the README in a new folder: init, the findings of
# examples/first-run, import, lint, and render. It fails when a command fails,
# when lint reports anything on the sample, when a file the README names is not
# made, when the PNG is not the one the README links to, or when a second
# import and render change a file.
#
#   scripts/first-run.sh [the ponchimap command]
#
# Without an argument it builds the command first, which may reach the
# network. The commands themselves do not, so CI runs this script with the
# network taken away.
set -eu
repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf -- "${work:?}"' EXIT

if [ $# -ge 1 ]; then
	tool=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
else
	tool="$work/ponchimap"
	(cd "$repo" && go build -o "$tool" ./cmd/ponchimap)
fi

mkdir "$work/app"
cd "$work/app"
"$tool" init
cp "$repo/examples/first-run/findings/"*.json .ponchimap/findings/
"$tool" import
said=$("$tool" lint 2>&1)
if [ "$said" != "0 errors, 0 warnings" ]; then
	printf 'first-run: lint reports something on the sample:\n%s\n' "$said" >&2
	exit 1
fi
"$tool" render

made="docs/architecture/overview.svg docs/architecture/overview.png .ponchimap/model.json .ponchimap/report.md"
for file in $made; do
	if [ ! -s "$file" ]; then
		echo "first-run: $file was not made" >&2
		exit 1
	fi
done
if ! cmp -s docs/architecture/overview.png "$repo/examples/first-run/overview.png"; then
	echo "first-run: the PNG differs from examples/first-run/overview.png" >&2
	exit 1
fi

mkdir "$work/first"
for file in $made; do
	cp "$file" "$work/first/$(basename "$file")"
done
"$tool" import
"$tool" render
for file in $made; do
	if ! cmp -s "$file" "$work/first/$(basename "$file")"; then
		echo "first-run: $file changed in the second run" >&2
		exit 1
	fi
done
echo "first-run: the figure and the report are made, and a second run changes nothing"
