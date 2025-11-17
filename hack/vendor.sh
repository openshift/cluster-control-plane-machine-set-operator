#!/bin/bash

set -e

echo "Updating dependencies for Control Plane Machine Set Operator workspace"

# Tidy all modules in the workspace
echo "Running go mod tidy for all modules..."
for module in . openshift-tests-extension; do
  if [ -f "$module/go.mod" ]; then
    echo "Tidying $module"
    (cd "$module" && GOWORK=off go mod tidy)
  fi
done

# Verify all modules
echo "Verifying all modules..."
for module in . openshift-tests-extension; do
  if [ -f "$module/go.mod" ]; then
    echo "Verifying $module"
    (cd "$module" && GOWORK=off go mod verify)
  fi
done

# Sync workspace
echo "Syncing Go workspace..."
go work sync

# Create unified vendor directory
echo "Creating unified vendor directory..."
go work vendor -v

# Also update submodule vendor for isolated builds
echo "Updating submodule vendor..."
(cd openshift-tests-extension && GOWORK=off go mod vendor)

echo "Done! All dependencies updated."
