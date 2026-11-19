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

# --- skills/byline/SKILL.md: one shared body, each copy's own frontmatter ---
# One fixed rule for this file only, because its slash-command form has to
# differ: take the body after the private copy's frontmatter, rewrite its
# bare "/byline " and "/byline`" command form to "/byline:byline " and
# "/byline:byline`", and keep this copy's own frontmatter (name, description,
# allowed-tools). scripts/skill-body.stamp.md holds the body exactly as
# staged, before the rewrite, so a test can check this copy's body is what
# the rule produces from it.
skill_md=skills/byline/SKILL.md
mkdir -p "$work/$(dirname "$skill_md")"
if ! git -C "$src_repo" show "$ref:$skill_md" >"$work/$skill_md" 2>/dev/null; then
	echo "sync-core.sh: $skill_md not found at $ref in $src_repo" >&2
	exit 1
fi
awk '/^---$/{c++; next} c>=2{print}' "$work/$skill_md" >"$work/.skill-body"
if grep -inwFf "$work/.terms" "$work/.skill-body" >"$work/.hits" 2>/dev/null; then
	while IFS= read -r hit; do
		echo "sync-core.sh: $skill_md body: $hit" >&2
	done <"$work/.hits"
	echo "sync-core.sh: source-only terms found in $skill_md body, nothing written" >&2
	exit 1
fi
sed -e 's#/byline #/byline:byline #g' -e 's#/byline`#/byline:byline`#g' \
	"$work/.skill-body" >"$work/.skill-body-public"
awk '/^---$/{c++} {print} c>=2{exit}' "$root/$skill_md" >"$work/.skill-frontmatter"
cat "$work/.skill-frontmatter" "$work/.skill-body-public" >"$root/$skill_md.tmp"
mv "$root/$skill_md.tmp" "$root/$skill_md"
cp "$work/.skill-body" "$root/scripts/skill-body.stamp.md"
echo "sync-core.sh: synced $skill_md's body from $src_repo@$ref"

# --- rules/core-rules.json: 49 shared rules, this copy's own last 22 ---
# One fixed rule for this file only, because a rule id is never changed or
# reused: take the private copy's file, keep this copy's own rules whose id
# doesn't appear in it (this copy's own 22-rule block), and fail if any
# other rule differs between the two files. scripts/core-rules-shared.stamp.json
# holds the 49 shared rules exactly as written, so a test can catch a direct
# edit here instead of through this script.
core_rules=rules/core-rules.json
shared_stamp="$root/scripts/core-rules-shared.stamp.json"
mkdir -p "$work/$(dirname "$core_rules")"
if ! git -C "$src_repo" show "$ref:$core_rules" >"$work/$core_rules" 2>/dev/null; then
	echo "sync-core.sh: $core_rules not found at $ref in $src_repo" >&2
	exit 1
fi
cat >"$work/.merge_core_rules.py" <<'PYEOF'
import json
import sys

src_path, dst_path = sys.argv[1], sys.argv[2]
with open(src_path, encoding="utf-8") as f:
	src = json.load(f)
with open(dst_path, encoding="utf-8") as f:
	dst = json.load(f)

shared_ids = {r["id"] for r in src["rules"] if not r["id"].startswith("BW")}
src_shared = [r for r in src["rules"] if r["id"] in shared_ids]
dst_shared = [r for r in dst["rules"] if r["id"] in shared_ids]
if src_shared != dst_shared:
	src_by_id = {r["id"]: r for r in src_shared}
	dst_by_id = {r["id"]: r for r in dst_shared}
	for rid in sorted(set(src_by_id) | set(dst_by_id)):
		if src_by_id.get(rid) != dst_by_id.get(rid):
			sys.stderr.write(f"sync-core.sh: {dst_path}: rule {rid} differs from the source\n")
	sys.exit(1)

dst_own = [r for r in dst["rules"] if r["id"] not in shared_ids]
merged = dict(src)
merged["rules"] = src_shared + dst_own
with open(dst_path, "w", encoding="utf-8") as f:
	f.write(json.dumps(merged, indent=2, ensure_ascii=False) + "\n")
with open(sys.argv[3], "w", encoding="utf-8") as f:
	f.write(json.dumps(src_shared, indent=2, ensure_ascii=False) + "\n")
PYEOF
if ! python3 -I "$work/.merge_core_rules.py" "$work/$core_rules" "$root/$core_rules" "$shared_stamp"; then
	echo "sync-core.sh: $core_rules: the shared rules differ from the source, nothing written" >&2
	exit 1
fi
echo "sync-core.sh: synced $core_rules's shared rules from $src_repo@$ref"

# Regenerate every embed/ copy from what was just written.
sh "$root/server/scripts/embed.sh"
