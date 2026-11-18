#!/bin/sh
# Copies the files on scripts/core-files.txt from the private copy this
# started from into this tree, and writes scripts/core-files.sha256.
# Never commits or pushes; leaves the result for a human to review.
set -eu

usage() {
	echo "usage: sync-core.sh <path-to-source-repo> [ref]" >&2
	exit 1
}

[ "$#" -ge 1 ] || usage
src_repo=$1
ref=${2:-origin/mainline}

script_dir=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$script_dir/.." && pwd)
list_file="$root/scripts/core-files.txt"
stamp_file="$root/scripts/core-files.sha256"

# Refuse on a dirty public working tree: a sync must land on a clean diff.
if [ -n "$(git -C "$root" status --porcelain)" ]; then
	echo "sync-core.sh: $root has uncommitted changes, refusing to run" >&2
	exit 1
fi

terms_file=${BYLINE_AMAZON_TERMS:-}
if [ -z "$terms_file" ] || [ ! -f "$terms_file" ]; then
	echo "sync-core.sh: BYLINE_AMAZON_TERMS must point at a terms file" >&2
	exit 1
fi

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

list=$(grep -vE '^[[:space:]]*(#|$)' "$list_file")

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Drop blank/comment lines: an empty grep -F pattern matches every line.
grep -vE '^[[:space:]]*(#|$)' "$terms_file" >"$work/.terms"

# Stage every listed file from the ref, read-only: never from $src_repo's
# working folder, which may hold work in progress.
echo "$list" | while IFS= read -r rel; do
	mkdir -p "$work/$(dirname "$rel")"
	if ! git -C "$src_repo" show "$ref:$rel" >"$work/$rel" 2>/dev/null; then
		echo "sync-core.sh: $rel not found at $ref in $src_repo" >&2
		exit 1
	fi
done

# Scan every staged file for source-only terms before writing anything.
# shortcut: plain case-insensitive whole-word/phrase matching, no per-term
# acronym rule like content/scrub_test.go's; widen if that causes a miss.
leak_found=0
echo "$list" | while IFS= read -r rel; do
	if grep -inwFf "$work/.terms" "$work/$rel" >"$work/.hits" 2>/dev/null; then
		while IFS= read -r hit; do
			echo "sync-core.sh: $rel: $hit" >&2
		done <"$work/.hits"
		echo leak >"$work/.leak"
	fi
done
if [ -f "$work/.leak" ]; then
	echo "sync-core.sh: source-only terms found, nothing written" >&2
	exit 1
fi

# Everything is clean. Write the files, then the stamp.
echo "$list" | while IFS= read -r rel; do
	mkdir -p "$root/$(dirname "$rel")"
	cp "$work/$rel" "$root/$rel"
done

: >"$stamp_file.tmp"
echo "$list" | while IFS= read -r rel; do
	printf '%s  %s\n' "$(sha256_of "$root/$rel")" "$rel" >>"$stamp_file.tmp"
done
mv "$stamp_file.tmp" "$stamp_file"

echo "sync-core.sh: synced $(echo "$list" | wc -l | tr -d ' ') files from $src_repo@$ref"
