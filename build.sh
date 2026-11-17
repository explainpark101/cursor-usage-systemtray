#!/usr/bin/env bash
# Compatibility wrapper — use build-mac.sh.
exec "$(cd "$(dirname "$0")" && pwd)/build-mac.sh" "$@"
