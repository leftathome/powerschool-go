# PowerShell script to run tests with credentials from 1Password
# This script should NOT contain actual 1Password paths - those should be in .env.local

param(
    [switch]$Headless,
    [switch]$Debug,
    [switch]$SaveHtml,
    [switch]$Help
)

if ($Help) {
    Write-Host "Usage: .\scripts\test-with-1password.ps1 [options]"
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  -Headless    Run browser in headless mode"
    Write-Host "  -Debug       Enable debug logging"
    Write-Host "  -SaveHtml    Save HTML responses to files"
    Write-Host "  -Help        Show this help message"
    Write-Host ""
    Write-Host "Credentials are loaded from .env.local file which should contain:"
    Write-Host "  POWERSCHOOL_URL=https://ps.example.org"
    Write-Host "  POWERSCHOOL_USERNAME_OP_PATH=op://vault/item/field"
    Write-Host "  POWERSCHOOL_PASSWORD_OP_PATH=op://vault/item/field"
    exit 0
}

# Check if .env.local exists
if (-not (Test-Path ".env.local")) {
    Write-Host "Error: .env.local file not found" -ForegroundColor Red
    Write-Host ""
    Write-Host "Create a .env.local file with the following content:"
    Write-Host "POWERSCHOOL_URL=https://ps.example.org"
    Write-Host "POWERSCHOOL_USERNAME_OP_PATH=op://Private/your-item/username"
    Write-Host "POWERSCHOOL_PASSWORD_OP_PATH=op://Private/your-item/password"
    Write-Host ""
    Write-Host "The actual 1Password paths should never be committed to git."
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
    Write-Host "Required variables:"
    Write-Host "  POWERSCHOOL_USERNAME_OP_PATH"
    Write-Host "  POWERSCHOOL_PASSWORD_OP_PATH"
    Write-Host ""
    Write-Host "Optional (can be hardcoded):"
    Write-Host "  POWERSCHOOL_URL or POWERSCHOOL_URL_OP_PATH"
    exit 1
}

# Check if op CLI is installed
if (-not (Get-Command op -ErrorAction SilentlyContinue)) {
    Write-Host "Error: 1Password CLI (op) is not installed or not in PATH" -ForegroundColor Red
    Write-Host "Install from: https://developer.1password.com/docs/cli/get-started/"
    exit 1
}

# Check if signed in to 1Password
try {
    op account list | Out-Null
} catch {
    Write-Host "Error: Not signed in to 1Password CLI" -ForegroundColor Red
    Write-Host "Run: op signin"
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
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}

# Validate credentials were fetched
if (-not $env:POWERSCHOOL_URL) {
    Write-Host "Error: POWERSCHOOL_URL not set (use POWERSCHOOL_URL or POWERSCHOOL_URL_OP_PATH in .env.local)" -ForegroundColor Red
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

# Build command flags
$flags = @()
if ($Headless) {
    $flags += "-headless"
}
if ($Debug) {
    $flags += "-debug"
}
if ($SaveHtml) {
    $flags += "-save-html"
}

Write-Host "Running authentication test..." -ForegroundColor Cyan
$flagsString = $flags -join " "
Write-Host "Command: go run ./cmd/test-auth $flagsString"
Write-Host ""

# Run the test
if ($flags.Count -gt 0) {
    go run ./cmd/test-auth @flags
} else {
    go run ./cmd/test-auth
}
