#!/usr/bin/env bash
# Build macOS cursor-tray for RunCat Neo, ad-hoc codesign, and pack an
# installable zip (binary + launchd plist + install helper).
#
# Usage:
#   ./build-mac.sh              # arm64 + amd64 + universal, sign, zip
#   ./build-mac.sh --install    # also install binary to ~/.local/bin
#   ./build-mac.sh --arch arm64 # only current-arch style single target
set -euo pipefail

root="$(cd "$(dirname "$0")" && pwd)"
cd "$root"

version="$(sed -nE 's/^\/\/ Version:[[:space:]]*([0-9.]+).*/\1/p' go.mod | head -1)"
version="${version:-0.0.0}"
out_dir="$root/dist"
mkdir -p "$out_dir"

do_install=0
only_arch=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --install) do_install=1 ;;
    --arch)
      shift
      only_arch="${1:-}"
      if [[ -z "$only_arch" ]]; then
        echo "error: --arch requires arm64 or amd64" >&2
        exit 2
      fi
      ;;
    -h|--help)
      sed -n '2,12p' "$0"
      exit 0
      ;;
    *)
      echo "unknown arg: $1" >&2
      exit 2
      ;;
  esac
  shift
done

if ! command -v go >/dev/null 2>&1; then
  echo "error: go not found in PATH. Install Go ${version:+(see go.mod)} and retry." >&2
  echo "  https://go.dev/dl/" >&2
  exit 1
fi
if ! command -v codesign >/dev/null 2>&1; then
  echo "error: codesign not found (install Xcode Command Line Tools)" >&2
  exit 1
fi

echo "==> go $(go version)"
echo "==> version $version"

LDFLAGS="-s -w -X cursor-tray/internal/version.Version=${version}"

build_one() {
  local arch="$1"
  local out="$out_dir/cursor-tray-darwin-${arch}-${version}"
  echo "==> build darwin/${arch}" >&2
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags="$LDFLAGS" -o "$out" ./cmd/cursor-tray
  echo "==> ad-hoc codesign ${out##*/}" >&2
  codesign --force --sign - --timestamp=none --identifier "dev.cursor-tray.runcat" "$out" >&2
  codesign --verify --verbose=2 "$out" >&2
  # Only the binary path goes to stdout (captured by caller).
  printf '%s\n' "$out"
}

signed_bins=()
if [[ -n "$only_arch" ]]; then
  case "$only_arch" in
    arm64|amd64) ;;
    *)
      echo "error: --arch must be arm64 or amd64" >&2
      exit 2
      ;;
  esac
  signed_bins+=("$(build_one "$only_arch")")
else
  signed_bins+=("$(build_one arm64)")
  signed_bins+=("$(build_one amd64)")
fi

universal=""
if [[ ${#signed_bins[@]} -eq 2 ]] && command -v lipo >/dev/null 2>&1; then
  universal="$out_dir/cursor-tray-darwin-universal-${version}"
  echo "==> lipo universal → ${universal##*/}"
  lipo -create -output "$universal" "${signed_bins[0]}" "${signed_bins[1]}"
  codesign --force --sign - --timestamp=none --identifier "dev.cursor-tray.runcat" "$universal"
  codesign --verify --verbose=2 "$universal" >&2
  signed_bins+=("$universal")
fi

# Prefer universal for the installable package; else host arch; else first.
host_arch="$(uname -m)"
case "$host_arch" in
  arm64|aarch64) prefer="arm64" ;;
  x86_64) prefer="amd64" ;;
  *) prefer="arm64" ;;
esac

pkg_bin=""
if [[ -n "$universal" && -f "$universal" ]]; then
  pkg_bin="$universal"
elif [[ -f "$out_dir/cursor-tray-darwin-${prefer}-${version}" ]]; then
  pkg_bin="$out_dir/cursor-tray-darwin-${prefer}-${version}"
else
  pkg_bin="${signed_bins[0]}"
fi

stage="$out_dir/stage-macos-${version}"
rm -rf "$stage"
mkdir -p "$stage/bin" "$stage/launchd"

cp "$pkg_bin" "$stage/bin/cursor-tray"
chmod 755 "$stage/bin/cursor-tray"
# Re-sign after copy (copy can break the seal on some volumes).
codesign --force --sign - --timestamp=none --identifier "dev.cursor-tray.runcat" "$stage/bin/cursor-tray"

if [[ -f "$root/launchd/dev.cursor-tray.runcat.plist" ]]; then
  cp "$root/launchd/dev.cursor-tray.runcat.plist" "$stage/launchd/"
fi

cat > "$stage/install.sh" <<'EOF'
#!/usr/bin/env bash
# Install cursor-tray into ~/.local/bin and optionally register launchd.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
prefix="${PREFIX:-$HOME/.local}"
bin_dir="$prefix/bin"
mkdir -p "$bin_dir"

cp "$here/bin/cursor-tray" "$bin_dir/cursor-tray"
chmod 755 "$bin_dir/cursor-tray"
codesign --force --sign - --timestamp=none --identifier "dev.cursor-tray.runcat" "$bin_dir/cursor-tray" >/dev/null 2>&1 || true

echo "installed: $bin_dir/cursor-tray"
echo "add to PATH if needed: export PATH=\"$bin_dir:\$PATH\""

if [[ "${1:-}" == "--launchd" ]]; then
  agents="$HOME/Library/LaunchAgents"
  mkdir -p "$agents"
  plist_src="$here/launchd/dev.cursor-tray.runcat.plist"
  plist_dst="$agents/dev.cursor-tray.runcat.plist"
  # Rewrite placeholder paths to this machine.
  sed \
    -e "s|/Users/YOU/.local/bin/cursor-tray|$bin_dir/cursor-tray|g" \
    -e "s|/Users/YOU|$HOME|g" \
    "$plist_src" > "$plist_dst"
  launchctl bootout "gui/$(id -u)/dev.cursor-tray.runcat" 2>/dev/null || true
  launchctl bootstrap "gui/$(id -u)" "$plist_dst"
  launchctl enable "gui/$(id -u)/dev.cursor-tray.runcat" 2>/dev/null || true
  launchctl kickstart -k "gui/$(id -u)/dev.cursor-tray.runcat" 2>/dev/null || \
    launchctl load "$plist_dst" 2>/dev/null || true
  echo "launchd loaded: $plist_dst"
fi

echo
echo "Next:"
echo "  1. cursor-tray set-cookie -f /path/to/cookies.txt"
echo "  2. cursor-tray once   # or: cursor-tray"
echo "  3. RunCat Neo → Settings → Metrics → Custom Metrics"
echo "     → ~/Library/Application Support/cursor-tray/runcat-usage.json"
EOF
chmod 755 "$stage/install.sh"

cat > "$stage/README.txt" <<EOF
cursor-tray ${version} (macOS / RunCat Neo)

Ad-hoc signed binary for local install.

Quick install:
  ./install.sh
  ./install.sh --launchd   # also start at login

Manual:
  cp bin/cursor-tray ~/.local/bin/
EOF

zip_path="$out_dir/cursor-tray-macos-${version}.zip"
rm -f "$zip_path"
echo "==> pack ${zip_path##*/}"
(
  cd "$out_dir"
  # Avoid AppleDouble/__MACOSX entries so the zip stays portable.
  COPYFILE_DISABLE=1 zip -qr "cursor-tray-macos-${version}.zip" "stage-macos-${version}"
)

# Convenient unversioned / host-arch symlink-style copies for local use
host_out="$out_dir/cursor-tray"
cp "$pkg_bin" "$host_out"
codesign --force --sign - --timestamp=none --identifier "dev.cursor-tray.runcat" "$host_out"
chmod 755 "$host_out"

if [[ "$do_install" -eq 1 ]]; then
  echo "==> install to ~/.local/bin"
  "$stage/install.sh"
fi

echo
echo "Artifacts:"
ls -lh "${signed_bins[@]}" "$host_out" "$zip_path" 2>/dev/null | awk '{print "  " $0}'
echo
echo "Installable zip: $zip_path"
echo "  unzip and run: ./install.sh   or   ./install.sh --launchd"
echo "Local binary:    $host_out"
