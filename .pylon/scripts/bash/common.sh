#!/bin/bash
set -euo pipefail

# Find repo root (walks up to find .pylon/)
find_repo_root() {
  local dir="$PWD"
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/.pylon" ]]; then
      echo "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  echo "ERROR: .pylon/ not found" >&2
  return 1
}

# JSON output helper (requires jq)
json_output() {
  if command -v jq &>/dev/null; then
    echo "$1" | jq .
  else
    echo "$1"
  fi
}

# Error handler
die() {
  echo "ERROR: $1" >&2
  exit 1
}

# Check required commands
require_cmd() {
  for cmd in "$@"; do
    command -v "$cmd" &>/dev/null || die "Required command not found: $cmd"
  done
}

# Returns 0 if the given branch name is a protected branch
readonly PROTECTED_BRANCHES=("main" "master" "develop" "dev")
is_protected_branch() {
  local branch="$1"
  for protected in "${PROTECTED_BRANCHES[@]}"; do
    [[ "$branch" == "$protected" ]] && return 0
  done
  return 1
}

REPO_ROOT="$(find_repo_root)"
PYLON_DIR="$REPO_ROOT/.pylon"
RUNTIME_DIR="$PYLON_DIR/runtime"
CONFIG_FILE="$PYLON_DIR/config.yml"

# config_get <dot.notation.key> [default]
# Reads a scalar value from .pylon/config.yml using dot-notation key path.
# Prints the value; prints default (or empty string) if key is absent or config is missing.
# Requires 'yq' (mikefarah or kislyuk) or 'python3' with PyYAML.
config_get() {
  local key="$1"
  local default="${2-}"
  local value=""

  if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "$default"
    return 0
  fi

  if command -v yq &>/dev/null; then
    value=$(yq ".$key" "$CONFIG_FILE" 2>/dev/null)
    [[ "$value" == "null" ]] && value=""
  elif command -v python3 &>/dev/null; then
    value=$(PYLON_CONFIG="$CONFIG_FILE" PYLON_KEY="$key" python3 -c "
import os, sys
try:
    import yaml
    with open(os.environ['PYLON_CONFIG']) as f:
        cfg = yaml.safe_load(f) or {}
    val = cfg
    for k in os.environ['PYLON_KEY'].split('.'):
        val = val.get(k) if isinstance(val, dict) else None
    print('' if val is None else str(val).lower(), end='')
except Exception:
    pass
" 2>/dev/null)
  else
    die "config_get: 'yq' 또는 'python3'이 필요합니다"
  fi

  echo "${value:-$default}"
}
