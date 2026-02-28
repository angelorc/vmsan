#!/usr/bin/env bash
set -euo pipefail

# Custom version script for alpha/beta pre-release versioning.
# Wraps `changeset version` to keep the base version fixed at 0.1.0
# while incrementing the pre-release counter.
#
# Current version format: 0.1.0-alpha.N
# Each run: consumes changesets, updates CHANGELOG.md, bumps N+1

PACKAGE_JSON="package.json"

# Read current version
current_version=$(node -p "require('./$PACKAGE_JSON').version")

# Extract pre-release tag and counter (e.g. "alpha" and "2" from "0.1.0-alpha.2")
if [[ "$current_version" =~ ^([0-9]+\.[0-9]+\.[0-9]+)-([a-z]+)\.([0-9]+)$ ]]; then
  base_version="${BASH_REMATCH[1]}"
  pre_tag="${BASH_REMATCH[2]}"
  pre_counter="${BASH_REMATCH[3]}"
else
  echo "Error: version '$current_version' does not match expected format (e.g. 0.1.0-alpha.0)"
  exit 1
fi

# Run changeset version (consumes .changeset files, updates CHANGELOG.md)
bunx changeset version

# Compute next pre-release version
next_counter=$((pre_counter + 1))
next_version="${base_version}-${pre_tag}.${next_counter}"

# Override the version in package.json
node -e "
const fs = require('fs');
const pkg = JSON.parse(fs.readFileSync('$PACKAGE_JSON', 'utf8'));
pkg.version = '$next_version';
fs.writeFileSync('$PACKAGE_JSON', JSON.stringify(pkg, null, 2) + '\n');
"

echo "Version bumped: $current_version -> $next_version"
