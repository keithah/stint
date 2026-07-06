const installScript = String.raw`#!/bin/sh
set -eu

repo="keithah/stint"
version="\${STINT_VERSION:-latest}"
install_dir="\${STINT_INSTALL_DIR:-$HOME/.local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  darwin|linux) ;;
  *) echo "Stint CLI is available for macOS and Linux. Detected: $os" >&2; exit 1 ;;
esac

case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "Stint CLI is available for amd64 and arm64. Detected: $arch" >&2; exit 1 ;;
esac

if [ "$version" = "latest" ]; then
  version="$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)"
fi

if [ -z "$version" ]; then
  echo "Could not find the latest Stint release." >&2
  exit 1
fi

asset="stint_\${version}_\${os}_\${arch}.tar.gz"
url="https://github.com/$repo/releases/download/$version/$asset"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$install_dir"
curl -fsSL "$url" -o "$tmp/$asset"
tar -xzf "$tmp/$asset" -C "$tmp"
install -m 0755 "$tmp/stint" "$install_dir/stint"

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "Add $install_dir to PATH to run stint from any terminal." ;;
esac

"$install_dir/stint" --version
echo "Stint CLI installed at $install_dir/stint"

if [ -n "\${STINT_API_URL:-}" ] && [ -n "\${STINT_API_KEY:-}" ]; then
  "$install_dir/stint" setup --server "$STINT_API_URL" --key "$STINT_API_KEY"
  "$install_dir/stint" doctor
else
  echo "Set STINT_API_URL and STINT_API_KEY to configure Stint during install."
fi
`.split("\\${").join("${");

export async function GET() {
  return new Response(installScript, {
    headers: {
      "content-type": "text/x-shellscript; charset=utf-8",
      "cache-control": "public, max-age=300",
    },
  });
}
