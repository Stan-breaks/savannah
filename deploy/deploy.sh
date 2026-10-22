#!/usr/bin/env bash
#
# Pulls the newest commit, rebuilds the API and restarts the service.
#
# The GitHub Actions workflow runs this over ssh.
set -euo pipefail

APP_DIR="${APP_DIR:-/var/www/savannah}"
CACHE_DIR="${CACHE_DIR:-/var/cache/savannah}"

cd "$APP_DIR"

# Keep the go caches out of the checkout.
export GOPATH="$CACHE_DIR/go"
export GOCACHE="$CACHE_DIR/go-build"

git pull --ff-only

# go build swaps the binary in with a rename, so it works even while the
# old one is still running.
go build -o savannah .

sudo systemctl restart savannah.service
