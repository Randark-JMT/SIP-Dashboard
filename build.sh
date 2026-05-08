#!/bin/bash
# build.sh - 在 FreePBX Linux 服务器上运行此脚本来构建项目
# 前提条件: Go >= 1.21, Node.js >= 18, libpcap-dev
#
# 安装依赖 (Debian/Ubuntu/CentOS based):
#   Debian/Ubuntu: sudo apt-get install -y libpcap-dev
#   CentOS/RHEL:   sudo yum install -y libpcap-devel

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$SCRIPT_DIR"
BACKEND_DIR="$PROJECT_ROOT/backend"
FRONTEND_DIR="$PROJECT_ROOT/frontend"
OUTPUT="$BACKEND_DIR/sip-dashboard"

echo "==> Building frontend..."
cd "$FRONTEND_DIR"
npm install
npm run build

echo "==> Building Go backend (with embedded frontend)..."
cd "$BACKEND_DIR"
go build -ldflags="-s -w" -o "$OUTPUT" ./cmd/server

echo ""
echo "Build complete: $OUTPUT"
echo ""
echo "Usage:"
echo "  # Give capture permissions (avoids needing sudo):"
echo "  sudo setcap cap_net_raw,cap_net_admin+eip ./sip-dashboard"
echo ""
echo "  # Run (replace eth0 with your actual network interface):"
echo "  ./sip-dashboard --interface eth0 --listen :8080"
echo ""
echo "  # To find your network interface:"
echo "  ip link show"
