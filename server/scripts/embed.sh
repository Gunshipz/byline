#!/bin/sh
set -eu

# the binary must carry the same files the plugin ships
script_dir=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$script_dir/../.." && pwd)

copy_one() {
	src=$1
	dst=$2
	rm -rf "$dst"
	if [ -e "$src" ]; then
		mkdir -p "$(dirname "$dst")"
		cp -R "$src" "$dst"
	fi
}

copy_one "$root/skills/byline" "$root/server/embed/skills/byline"
# the helper never runs the evals; they ship only in the skill folder itself
rm -rf "$root/server/embed/skills/byline/evals"
copy_one "$root/rules" "$root/server/embed/rules"
copy_one "$root/templates" "$root/server/embed/templates"
