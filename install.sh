#!/usr/bin/env bash
set -euo pipefail

if [[ "$EUID" -ne 0 ]]; then
  echo "Please run as root: sudo bash install.sh"
  exit 1
fi

echo "[TunnelZero] Preparing system..."
apt-get update
apt-get install -y curl wget ca-certificates unzip git

if ! command -v go >/dev/null 2>&1; then
  echo "[TunnelZero] Installing Go 1.22.4..."
  curl -fsSL -o /tmp/go1.22.4.linux-amd64.tar.gz https://go.dev/dl/go1.22.4.linux-amd64.tar.gz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf /tmp/go1.22.4.linux-amd64.tar.gz
  echo 'export PATH=$PATH:/usr/local/go/bin' >/etc/profile.d/go.sh
  export PATH=$PATH:/usr/local/go/bin
fi

echo "[TunnelZero] Building application..."
cd "$(dirname "$0")"
/usr/local/go/bin/go build -o tunnelzero

echo "[TunnelZero] Starting installer..."
./tunnelzero
