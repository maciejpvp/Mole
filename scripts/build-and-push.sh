#!/usr/bin/env bash

# ==============================================================================
# Mole — Build & Push Production Images
#
# Run this from a DEVELOPMENT machine, never on the production server: the
# frontend build runs `npm ci` + vite, which will not survive a 2 GB instance.
#
# IMPORTANT: the nginx configs (nginx/nginx.conf, nginx/conf.d/, nginx/partials/,
# nginx/docker-entrypoint.d/) are baked INTO the mole-nginx image. Editing those
# files on the production host does nothing — any change under nginx/ requires
# rebuilding and pushing that image with this script.
#
#   ./scripts/build-and-push.sh                 # tag from `git describe`
#   ./scripts/build-and-push.sh v1.2.3          # explicit tag
#   ./scripts/build-and-push.sh --only nginx    # rebuild one image
# ==============================================================================

set -euo pipefail

info()  { echo -e "\e[32m[INFO]\e[0m  $1"; }
warn()  { echo -e "\e[33m[WARN]\e[0m  $1"; }
error() { echo -e "\e[31m[ERROR]\e[0m $1"; exit 1; }

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

REGISTRY_NAMESPACE="${REGISTRY_NAMESPACE:-maciekpvp}"
PLATFORM="${PLATFORM:-linux/amd64}"

TAG=""
ONLY=""
ALLOW_DIRTY=0

while [ $# -gt 0 ]; do
  case "$1" in
    --only)
      [ $# -ge 2 ] || error "--only requires a value: nginx | frontend | control-plane"
      ONLY="$2"
      shift 2
      ;;
    --allow-dirty)
      ALLOW_DIRTY=1
      shift
      ;;
    -h|--help)
      sed -n '3,17p' "${BASH_SOURCE[0]}"
      exit 0
      ;;
    -*)
      error "Unknown flag: $1"
      ;;
    *)
      [ -z "$TAG" ] || error "Tag already set to '$TAG'; unexpected argument '$1'"
      TAG="$1"
      shift
      ;;
  esac
done

case "$ONLY" in
  ""|nginx|frontend|control-plane) ;;
  *) error "--only must be one of: nginx | frontend | control-plane (got '$ONLY')" ;;
esac

# ------------------------------------------------------------------------------
# Tag resolution — a tag must correspond to a real commit
# ------------------------------------------------------------------------------
if [ -n "$(git status --porcelain)" ]; then
  if [ "$ALLOW_DIRTY" -eq 1 ]; then
    warn "Working tree is dirty; continuing because --allow-dirty was passed."
  else
    error "Working tree is dirty. Commit your changes, or pass --allow-dirty."
  fi
fi

if [ -z "$TAG" ]; then
  TAG="$(git describe --tags --always --dirty)"
fi
info "Building tag: $TAG (platform $PLATFORM, namespace $REGISTRY_NAMESPACE)"

# ------------------------------------------------------------------------------
# Stripe publishable key — baked into the frontend bundle at build time.
# Never sourced from the production .env: that file holds the SECRET key.
# ------------------------------------------------------------------------------
if [ -z "${VITE_STRIPE_PUBLISHABLE_KEY:-}" ] && [ -f "$REPO_ROOT/.env.build" ]; then
  info "Loading VITE_STRIPE_PUBLISHABLE_KEY from .env.build"
  # shellcheck disable=SC1091
  set -a; . "$REPO_ROOT/.env.build"; set +a
fi

needs_frontend=0
if [ -z "$ONLY" ] || [ "$ONLY" = "frontend" ]; then
  needs_frontend=1
fi

if [ "$needs_frontend" -eq 1 ] && [ -z "${VITE_STRIPE_PUBLISHABLE_KEY:-}" ]; then
  error "VITE_STRIPE_PUBLISHABLE_KEY is not set. Export it, or create $REPO_ROOT/.env.build containing:
    VITE_STRIPE_PUBLISHABLE_KEY=pk_live_..."
fi

command -v docker >/dev/null 2>&1 || error "docker is not installed."
docker buildx version >/dev/null 2>&1 || error "docker buildx is required (it sets --platform explicitly, so an arm64 workstation cannot ship images that fail with 'exec format error' on an amd64 server)."

# ------------------------------------------------------------------------------
# Builds
# ------------------------------------------------------------------------------
build_image() {
  local name="$1" context="$2"; shift 2
  local repo="${REGISTRY_NAMESPACE}/${name}"

  info "Building ${repo}:${TAG} from ${context}"
  docker buildx build \
    --platform "$PLATFORM" \
    -t "${repo}:${TAG}" \
    -t "${repo}:latest" \
    --push \
    "$@" \
    "$context"
  info "Pushed ${repo}:${TAG} and ${repo}:latest"
}

if [ -z "$ONLY" ] || [ "$ONLY" = "nginx" ]; then
  build_image mole-nginx nginx
fi

if [ -z "$ONLY" ] || [ "$ONLY" = "control-plane" ]; then
  build_image mole-control-plane control-plane/mole-control-plane
fi

if [ "$needs_frontend" -eq 1 ]; then
  build_image mole-frontend mole-frontend \
    --build-arg "VITE_STRIPE_PUBLISHABLE_KEY=${VITE_STRIPE_PUBLISHABLE_KEY}"
fi

# ------------------------------------------------------------------------------
# Done — paste the last line into the production host's .env
# ------------------------------------------------------------------------------
echo
info "All images pushed. Pin this tag in the production .env, then run:"
info "  docker compose -f docker-compose.prod.yml up -d"
echo
echo "MOLE_IMAGE_TAG=$TAG"
