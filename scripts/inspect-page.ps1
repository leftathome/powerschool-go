#!/usr/bin/env pwsh
# Script to run the page inspection tool with 1Password credentials

# Check if .env.local exists
if (-not (Test-Path ".env.local")) {
    Write-Host "Error: .env.local file not found" -ForegroundColor Red
    exit 1
}

# Load environment variables from .env.local
Write-Host "Loading 1Password configuration from .env.local..." -ForegroundColor Cyan
Get-Content .env.local | ForEach-Object {
    if ($_ -match '^\s*([^#][^=]+)=(.*)$') {
        $key = $matches[1].Trim()
        $value = $matches[2].Trim()
        Set-Item -Path "env:$key" -Value $value
    }
}

# Validate that required variables are set
if (-not $env:POWERSCHOOL_USERNAME_OP_PATH -or -not $env:POWERSCHOOL_PASSWORD_OP_PATH) {
    Write-Host "Error: Missing 1Password paths in .env.local" -ForegroundColor Red
    exit 1
}

# Check if op CLI is installed
if (-not (Get-Command op -ErrorAction SilentlyContinue)) {
    Write-Host "Error: 1Password CLI (op) is not installed or not in PATH" -ForegroundColor Red
    exit 1
}

Write-Host "Fetching credentials from 1Password..." -ForegroundColor Cyan

# Fetch URL (either from 1Password or use hardcoded value)
if ($env:POWERSCHOOL_URL_OP_PATH) {
    try {
        $env:POWERSCHOOL_URL = op read $env:POWERSCHOOL_URL_OP_PATH
    } catch {
        Write-Host "Warning: Failed to read URL from 1Password, using hardcoded value if available" -ForegroundColor Yellow
    }
}

# Fetch username and password from 1Password
try {
    $env:POWERSCHOOL_USERNAME = op read $env:POWERSCHOOL_USERNAME_OP_PATH
    $env:POWERSCHOOL_PASSWORD = op read $env:POWERSCHOOL_PASSWORD_OP_PATH
} catch {
    Write-Host "Error: Failed to fetch credentials from 1Password" -ForegroundColor Red
    exit 1
}

# Validate credentials were fetched
if (-not $env:POWERSCHOOL_URL) {
    Write-Host "Error: POWERSCHOOL_URL not set" -ForegroundColor Red
    exit 1
}
if (-not $env:POWERSCHOOL_USERNAME -or -not $env:POWERSCHOOL_PASSWORD) {
    Write-Host "Error: Failed to fetch username or password from 1Password" -ForegroundColor Red
    exit 1
}

Write-Host "Credentials loaded successfully" -ForegroundColor Green
Write-Host "URL: $env:POWERSCHOOL_URL"
Write-Host "Username: $env:POWERSCHOOL_USERNAME"
Write-Host ""

Write-Host "Starting page inspection tool..."
Write-Host "The browser will open and navigate to the scores page."
Write-Host ""

# Run the inspection tool
.\inspect-page.exe
