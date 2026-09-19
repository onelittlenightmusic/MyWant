#!/usr/bin/env bash
# Run fmtool from source, rooted where you called it from.
#
# The --root is what the file tools are allowed to see, and the useful answer to
# "what is in this directory" is about the directory you are standing in, not
# the one this package lives in.
set -euo pipefail
orig_dir="$(pwd)"
cd "$(dirname "$0")"
swift run --quiet fmtool --root "$orig_dir" "$@"
