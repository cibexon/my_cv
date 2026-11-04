#!/usr/bin/env bash
# Copies the wiki markdown from interview-prep into content/; commit and push → CI builds a new image.
set -euo pipefail
cd "$(dirname "$0")/.."
src=${1:-$HOME/work/GitHubRepos/interview-prep/topics}

rsync -a --delete --include='*.md' --include='en/' --exclude='*' "$src/" content/
git status --short content/
