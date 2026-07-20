#!/usr/bin/env bash

# ==============================================================================
# Mole Server - Automated Installation Script
# ==============================================================================

set -euo pipefail

# --- CONFIGURATION ---
INSTALL_DIR="/opt/mole-server"
COMPOSE_URL="https://raw.githubusercontent.com/maciejpvp/Mole/main/server/docker-compose.yml"

# --- HELPER FUNCTIONS ---
info()  { echo -e "\e[32m[INFO]\e[0m $1"; }
warn()  { echo -e "\e[33m[WARN]\e[0m $1"; }
error() { echo -e "\e[31m[ERROR]\e[0m $1"; exit 1; }

# --- PRE-FLIGHT CHECKS ---
if [ "$EUID" -ne 0 ]; then
  error "This script must be run as root (or with sudo) to configure /opt/ and install Docker."
fi

if ! command -v curl >/dev/null 2>&1; then
  error "curl is required but not installed. Please install it via your package manager."
fi

# --- DOCKER SETUP ---
if ! command -v docker >/dev/null 2>&1; then
  info "Docker is not installed. Fetching official installation script..."
  curl -fsSL https://get.docker.com | sh
  info "Docker installed successfully."
  systemctl enable --now docker
else
  info "Docker is already installed."
fi

# Determine the correct Docker Compose command
if docker compose version >/dev/null 2>&1; then
  DOCKER_COMPOSE_CMD="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  DOCKER_COMPOSE_CMD="docker-compose"
else
  warn "Docker Compose plugin not found. Installing via package manager..."
  curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg 2>/dev/null
  apt-get update -y && apt-get install -y docker-compose-plugin || yum install -y docker-compose-plugin
  DOCKER_COMPOSE_CMD="docker compose"
fi

# --- DIRECTORY SETUP ---
info "Setting up installation directory at $INSTALL_DIR..."
mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"

# --- USER INPUT ---
echo ""
info "--- Mole Server Configuration ---"

# Pre-initialize variables so set -u doesn't trigger when piped or on EOF
INPUT_SECRET=""
INPUT_IMAGE=""
INPUT_CONTROL=""
INPUT_PUBLIC=""

# 1. MOLE_SECRET
DEFAULT_SECRET=$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p)
read -r -p "Enter MOLE_SECRET [Press enter to auto-generate a secure token]: " INPUT_SECRET </dev/tty || true
MOLE_SECRET="${INPUT_SECRET:-$DEFAULT_SECRET}"

# 2. MOLE_IMAGE
read -r -p "Enter MOLE_IMAGE [maciekpvp/mole-server:latest]: " INPUT_IMAGE </dev/tty || true
MOLE_IMAGE="${INPUT_IMAGE:-maciekpvp/mole-server:latest}"

# 3. CONTROL_PORT
read -r -p "Enter CONTROL_PORT [9000]: " INPUT_CONTROL </dev/tty || true
CONTROL_PORT="${INPUT_CONTROL:-9000}"

# 4. PUBLIC_PORT
read -r -p "Enter PUBLIC_PORT [8000]: " INPUT_PUBLIC </dev/tty || true
PUBLIC_PORT="${INPUT_PUBLIC:-8000}"

# --- GENERATE .env ---
info "Generating .env file..."
cat <<EOF > .env
# Mole Server Environment Configuration
# Generated on $(date)

# Shared secret — clients must supply this with --secret to connect.
MOLE_SECRET=$MOLE_SECRET

# Registry image to pull.
MOLE_IMAGE=$MOLE_IMAGE

# Exposed Ports
CONTROL_PORT=$CONTROL_PORT
PUBLIC_PORT=$PUBLIC_PORT
EOF

# Lock down .env permissions so only root can read the secret
chmod 600 .env
info ".env file secured (chmod 600)."

# --- FETCH DOCKER COMPOSE ---
info "Fetching docker-compose.yml..."
curl -fSL "$COMPOSE_URL" -o docker-compose.yml || error "Failed to download docker-compose.yml from $COMPOSE_URL"

# --- DEPLOY ---
info "Pulling latest image and starting Mole Server..."
$DOCKER_COMPOSE_CMD pull
$DOCKER_COMPOSE_CMD up -d --remove-orphans

echo ""
info "============================================================"
info "Installation complete! Mole Server is now running in the background."
info "Directory: $INSTALL_DIR"
info "To view logs, run: cd $INSTALL_DIR && $DOCKER_COMPOSE_CMD logs -f"
info "============================================================"