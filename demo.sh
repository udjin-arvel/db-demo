#!/usr/bin/env sh
set -e
ROOT=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
cd "$ROOT"
case "${1:-seed}" in
  seed) shift; exec go run ./backend/cmd/seed "$@" ;;
  *) echo "Unknown command: ${1:-}" >&2; exit 1 ;;
esac
