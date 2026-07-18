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

# Convert arguments to a JSON string array (requires jq). No arguments → []
# Call with the bash-3.2-safe empty-array expansion: array_to_json ${arr[@]+"${arr[@]}"}
array_to_json() {
  jq -cn '$ARGS.positional' --args "$@"
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

# extract_arg <arg-name> [args...] — prints value of --<arg-name>, empty if not found
extract_arg() {
  local name="$1"; shift
  local prev=""
  for arg in "$@"; do
    [[ "$prev" == "--$name" ]] && { echo "$arg"; return 0; }
    prev="$arg"
  done
}

# resolve_git_root [path] — sets GIT_ROOT. Priority: arg > config git.repo > git rev-parse
resolve_git_root() {
  local override="${1:-}"
  if [[ -n "$override" ]]; then
    [[ "$override" == --* ]] && die "--git-root 값이 잘못 지정됨: '$override'. 사용법: --git-root <repo-rel-path>"
    [[ -d "$REPO_ROOT/$override" ]] || die "--git-root '$override' 경로가 존재하지 않습니다: $REPO_ROOT/$override"
    GIT_ROOT="$(realpath "$REPO_ROOT/$override")"
  else
    local cfg
    cfg=$(config_get "git.repo" "")
    if [[ -n "$cfg" ]]; then
      [[ -d "$REPO_ROOT/$cfg" ]] || die "config git.repo '$cfg' 경로가 존재하지 않습니다: $REPO_ROOT/$cfg"
      GIT_ROOT="$(realpath "$REPO_ROOT/$cfg")"
    else
      GIT_ROOT="$(git -C "$REPO_ROOT" rev-parse --show-toplevel 2>/dev/null || echo "$REPO_ROOT")"
    fi
  fi
}

resolve_git_root
