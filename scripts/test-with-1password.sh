#!/bin/bash

# Script to run tests with credentials from 1Password
# This script should NOT contain actual 1Password paths - those should be in .env.local

set -e

# Default values
HEADLESS=false
DEBUG=false
SAVE_HTML=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --headless)
      HEADLESS=true
      shift
      ;;
    --debug)
      DEBUG=true
      shift
      ;;
    --save-html)
      SAVE_HTML=true
      shift
      ;;
    --help)
      echo "Usage: $0 [options]"
      echo ""
      echo "Options:"
      echo "  --headless    Run browser in headless mode"
      echo "  --debug       Enable debug logging"
      echo "  --save-html   Save HTML responses to files"
      echo "  --help        Show this help message"
      echo ""
      echo "Credentials are loaded from .env.local file which should contain:"
      echo "  POWERSCHOOL_URL_OP_PATH=op://vault/item/field"
      echo "  POWERSCHOOL_USERNAME_OP_PATH=op://vault/item/field"
      echo "  POWERSCHOOL_PASSWORD_OP_PATH=op://vault/item/field"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help for usage information"
      exit 1
      ;;
  esac
done

# Check if .env.local exists
if [ ! -f .env.local ]; then
  echo "Error: .env.local file not found"
  echo ""
  echo "Create a .env.local file with the following content:"
  echo "POWERSCHOOL_URL_OP_PATH=op://Private/your-item/url"
  echo "POWERSCHOOL_USERNAME_OP_PATH=op://Private/your-item/username"
  echo "POWERSCHOOL_PASSWORD_OP_PATH=op://Private/your-item/password"
  echo ""
  echo "The actual 1Password paths should never be committed to git."
  exit 1
fi

# Load 1Password paths from .env.local
echo "Loading 1Password configuration from .env.local..."
source .env.local

# Validate that required variables are set
if [ -z "$POWERSCHOOL_USERNAME_OP_PATH" ] || [ -z "$POWERSCHOOL_PASSWORD_OP_PATH" ]; then
  echo "Error: Missing 1Password paths in .env.local"
  echo "Required variables:"
  echo "  POWERSCHOOL_USERNAME_OP_PATH"
  echo "  POWERSCHOOL_PASSWORD_OP_PATH"
  echo ""
  echo "Optional (can be hardcoded):"
  echo "  POWERSCHOOL_URL or POWERSCHOOL_URL_OP_PATH"
  exit 1
fi

# Check if op CLI is installed
if ! command -v op &> /dev/null; then
  echo "Error: 1Password CLI (op) is not installed or not in PATH"
  echo "Install from: https://developer.1password.com/docs/cli/get-started/"
  exit 1
fi

# Check if signed in to 1Password
if ! op account list &> /dev/null; then
  echo "Error: Not signed in to 1Password CLI"
  echo "Run: eval \$(op signin)"
  exit 1
fi

echo "Fetching credentials from 1Password..."

# Fetch URL (either from 1Password or use hardcoded value)
if [ -n "$POWERSCHOOL_URL_OP_PATH" ]; then
  export POWERSCHOOL_URL=$(op read "$POWERSCHOOL_URL_OP_PATH")
fi

# Fetch username and password from 1Password
export POWERSCHOOL_USERNAME=$(op read "$POWERSCHOOL_USERNAME_OP_PATH")
export POWERSCHOOL_PASSWORD=$(op read "$POWERSCHOOL_PASSWORD_OP_PATH")

# Validate credentials were fetched
if [ -z "$POWERSCHOOL_URL" ]; then
  echo "Error: POWERSCHOOL_URL not set (use POWERSCHOOL_URL or POWERSCHOOL_URL_OP_PATH in .env.local)"
  exit 1
fi
if [ -z "$POWERSCHOOL_USERNAME" ] || [ -z "$POWERSCHOOL_PASSWORD" ]; then
  echo "Error: Failed to fetch username or password from 1Password"
  exit 1
fi

echo "Credentials loaded successfully"
echo "URL: $POWERSCHOOL_URL"
echo "Username: $POWERSCHOOL_USERNAME"
echo ""

# Build flags for test-auth
FLAGS=""
if [ "$HEADLESS" = true ]; then
  FLAGS="$FLAGS -headless"
fi
if [ "$DEBUG" = true ]; then
  FLAGS="$FLAGS -debug"
fi
if [ "$SAVE_HTML" = true ]; then
  FLAGS="$FLAGS -save-html"
fi

echo "Running authentication test..."
echo "Command: go run ./cmd/test-auth $FLAGS"
echo ""

# Run the test
go run ./cmd/test-auth $FLAGS
