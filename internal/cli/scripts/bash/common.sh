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
PROTECTED_BRANCHES=("main" "master" "develop" "dev")
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
