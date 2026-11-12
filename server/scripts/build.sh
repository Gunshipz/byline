#!/bin/sh
set -eu

# GOTOOLCHAIN=local stops Go downloading a toolchain inside Brazil, and
# -buildvcs=false stops the build failing on a checkout without git.

script_dir=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$script_dir/../.." && pwd)
server_dir="$root/server"

outdir=${1:?usage: build.sh OUTDIR}
mkdir -p "$outdir"
outdir=$(cd "$outdir" && pwd)

go_bin=${GO:-go}
if ! command -v "$go_bin" >/dev/null 2>&1; then
	echo "Go 1.22 or later is needed to build byline-mcp" >&2
	exit 1
fi

for name in skills/byline rules templates; do
	if [ ! -e "$root/$name" ]; then
		echo "$name/ is missing" >&2
		exit 1
	fi
done

sh "$script_dir/embed.sh"

rm -f "$outdir"/byline-mcp-* "$outdir/byline-mcp.exe"

version=$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$root/skills/byline/skill.json")

build_one() {
	goos=$1
	goarch=$2
	out=$3
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOTOOLCHAIN=local \
		"$go_bin" build -trimpath -buildvcs=false -ldflags "-s -w" -o "$out" ./cmd/byline-mcp
}

(
	cd "$server_dir"
	build_one darwin arm64 "$outdir/byline-mcp-darwin-arm64"
	build_one darwin amd64 "$outdir/byline-mcp-darwin-amd64"
	build_one linux amd64 "$outdir/byline-mcp-linux-amd64"
	build_one linux arm64 "$outdir/byline-mcp-linux-arm64"
	build_one windows amd64 "$outdir/byline-mcp.exe"
)

host_goos=$("$go_bin" env GOOS)
host_goarch=$("$go_bin" env GOARCH)
host_bin=""
case "$host_goos/$host_goarch" in
	darwin/arm64) host_bin="$outdir/byline-mcp-darwin-arm64" ;;
	darwin/amd64) host_bin="$outdir/byline-mcp-darwin-amd64" ;;
	linux/amd64) host_bin="$outdir/byline-mcp-linux-amd64" ;;
	linux/arm64) host_bin="$outdir/byline-mcp-linux-arm64" ;;
	windows/amd64) host_bin="$outdir/byline-mcp.exe" ;;
esac

if [ -n "$host_bin" ]; then
	got=$("$host_bin" version)
	want="byline-mcp $version"
	if [ "$got" != "$want" ]; then
		echo "host build check failed: got '$got', want '$want'" >&2
		exit 1
	fi
fi
