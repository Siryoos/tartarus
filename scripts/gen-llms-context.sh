#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(git rev-parse --show-toplevel)
cd "$PROJECT_ROOT"

python3 scripts/gen-llms-context.py
