#!/usr/bin/env bash
set -euo pipefail
orig_dir="$(pwd)"
cd "$(dirname "$0")/swift"
swift run --quiet fmtool --root "$orig_dir" "$@"
