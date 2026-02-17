#!/usr/bin/env bash
set -euo pipefail

export GOTOOLCHAIN=auto
BINARY="gdoc2md"
DIST="dist"

# Require a version argument.
if [ $# -lt 1 ]; then
  echo "Usage: ./release.sh <version> [--draft]"
  echo "Example: ./release.sh v1.0.0"
  exit 1
fi

VERSION="$1"
DRAFT_FLAG=""
if [ "${2:-}" = "--draft" ]; then
  DRAFT_FLAG="--draft"
fi

# Validate version format.
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Error: version must be in the format v0.0.0"
  exit 1
fi

# Ensure working tree is clean.
if [ -n "$(git status --porcelain)" ]; then
  echo "Error: working tree is not clean. Commit or stash changes first."
  exit 1
fi

# Ensure we're on main.
BRANCH=$(git branch --show-current)
if [ "$BRANCH" != "main" ]; then
  echo "Warning: you are on branch '$BRANCH', not 'main'. Continue? [y/N]"
  read -r CONFIRM
  if [ "$CONFIRM" != "y" ] && [ "$CONFIRM" != "Y" ]; then
    exit 1
  fi
fi

echo "==> Building $VERSION for all platforms..."

LDFLAGS="-s -w -X main.version=${VERSION}"
GOFLAGS="-trimpath"
rm -rf "$DIST"
mkdir -p "$DIST"

PLATFORMS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
  "windows/arm64"
)

for PLATFORM in "${PLATFORMS[@]}"; do
  GOOS="${PLATFORM%/*}"
  GOARCH="${PLATFORM#*/}"
  OUTPUT="${DIST}/${BINARY}-${GOOS}-${GOARCH}"
  if [ "$GOOS" = "windows" ]; then
    OUTPUT="${OUTPUT}.exe"
  fi
  echo "    Building ${GOOS}/${GOARCH}..."
  GOOS="$GOOS" GOARCH="$GOARCH" go build $GOFLAGS -ldflags "$LDFLAGS" -o "$OUTPUT" .
done

echo "==> Creating checksums..."
(cd "$DIST" && shasum -a 256 * > checksums.txt)

echo "==> Tagging ${VERSION}..."
git tag -a "$VERSION" -m "Release ${VERSION}"
git push origin "$VERSION"

echo "==> Creating GitHub release ${VERSION}..."
gh release create "$VERSION" \
  --title "$VERSION" \
  --generate-notes \
  $DRAFT_FLAG \
  ${DIST}/*

echo "==> Done! Release ${VERSION} created."
echo "    https://github.com/$(gh repo view --json nameWithOwner -q .nameWithOwner)/releases/tag/${VERSION}"
