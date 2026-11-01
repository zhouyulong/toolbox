#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if ! command -v wails >/dev/null 2>&1; then
  echo 'Install Wails CLI v2.10.2 and add the Go bin directory to PATH.' >&2
  exit 1
fi
case "$(uname -s)" in
  Linux) wails build -clean -tags webkit2_41 "$@" ;;
  Darwin) wails build -clean "$@" ;;
  *) echo 'Use scripts/build.ps1 on Windows.' >&2; exit 1 ;;
esac
