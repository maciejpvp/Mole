#!/usr/bin/env bash

# ==============================================================================
# Mole — EC2 Server Setup Script
# Run once as root before deploying: sudo bash scripts/setup-server.sh
# ==============================================================================

set -euo pipefail

info()  { echo -e "\e[32m[INFO]\e[0m  $1"; }
warn()  { echo -e "\e[33m[WARN]\e[0m  $1"; }
error() { echo -e "\e[31m[ERROR]\e[0m $1"; exit 1; }

if [ "$EUID" -ne 0 ]; then
  error "Run this script as root: sudo bash scripts/setup-server.sh"
fi

# ------------------------------------------------------------------------------
# 1. SWAP — 2 GB
# ------------------------------------------------------------------------------
if swapon --show | grep -q /swapfile; then
  warn "Swapfile already active, skipping."
else
  info "Creating 2 GB swapfile at /swapfile..."
  fallocate -l 2G /swapfile
  chmod 600 /swapfile
  mkswap /swapfile
  swapon /swapfile

  # Persist across reboots
  if ! grep -q '/swapfile' /etc/fstab; then
    echo '/swapfile none swap sw 0 0' >> /etc/fstab
  fi

  # Tune swappiness — only use swap under real pressure
  sysctl vm.swappiness=10
  if ! grep -q 'vm.swappiness' /etc/sysctl.conf; then
    echo 'vm.swappiness=10' >> /etc/sysctl.conf
  fi

  info "Swap enabled: $(free -h | grep Swap)"
fi

# ------------------------------------------------------------------------------
# 2. Docker daemon — safe IP pool (avoids AWS VPC 172.31.x.x collision)
# ------------------------------------------------------------------------------
DAEMON_JSON=/etc/docker/daemon.json

if [ -f "$DAEMON_JSON" ] && grep -q '10.200.0.0' "$DAEMON_JSON"; then
  warn "Docker daemon.json already configured, skipping."
else
  info "Writing $DAEMON_JSON with safe address pool..."
  mkdir -p /etc/docker
  cat > "$DAEMON_JSON" <<'EOF'
{
  "default-address-pools": [
    {
      "base": "10.200.0.0/16",
      "size": 24
    }
  ]
}
EOF

  info "Restarting Docker daemon..."
  systemctl restart docker
  info "Docker restarted."
fi

# ------------------------------------------------------------------------------
# Done
# ------------------------------------------------------------------------------
info "Setup complete. You can now run:"
info "  docker compose -f docker-compose.prod.yml up -d"
