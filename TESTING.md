# Testing Guide

This document describes how to test the powerschool-go library against your PowerSchool instance.

## Prerequisites

1. **Go 1.23+** installed
2. **1Password CLI** (`op`) installed and configured
3. **Chrome/Chromium** browser installed (for chromedp)
4. **Your PowerSchool credentials** stored in 1Password

## Initial Setup

### 1. Install 1Password CLI

If not already installed:
```bash
# macOS
brew install --cask 1password-cli

# Windows
winget install 1Password.CLI

# Or download from: https://developer.1password.com/docs/cli/get-started/
```

### 2. Sign in to 1Password CLI

```bash
eval $(op signin)
```

### 3. Verify Your 1Password Paths

The `.env.local` file is already configured with your PowerSchool credentials:
- Username: `op://Private/your-powerschool-item/username`
- Password: `op://Private/your-powerschool-item/password`
- URL: `https://ps.example.org` (hardcoded)

Verify you can access them:
```bash
op read "op://Private/your-powerschool-item/username"
op read "op://Private/your-powerschool-item/password"
```

## Running Tests

### Option 1: Using the Helper Script (Recommended)

The helper script automatically fetches credentials from 1Password:

```bash
# Make the script executable (Linux/macOS)
chmod +x scripts/test-with-1password.sh

# Run with visible browser (recommended for first test)
./scripts/test-with-1password.sh

# Run with visible browser and debug logging
./scripts/test-with-1password.sh --debug

# Run in headless mode (no visible browser)
./scripts/test-with-1password.sh --headless

# Run with HTML saving (for debugging parsers)
./scripts/test-with-1password.sh --debug --save-html
```

### Option 2: Manual Testing

Set environment variables and run directly:

```bash
# Fetch credentials
export POWERSCHOOL_URL="https://ps.example.org"
export POWERSCHOOL_USERNAME=$(op read "op://Private/your-powerschool-item/username")
export POWERSCHOOL_PASSWORD=$(op read "op://Private/your-powerschool-item/password")

# Run the test
go run ./cmd/test-auth

# Or with flags
go run ./cmd/test-auth -debug
go run ./cmd/test-auth -headless
```

### Option 3: Windows PowerShell

```powershell
# Set environment variables
$env:POWERSCHOOL_URL = "https://ps.example.org"
$env:POWERSCHOOL_USERNAME = op read "op://Private/your-powerschool-item/username"
$env:POWERSCHOOL_PASSWORD = op read "op://Private/your-powerschool-item/password"

# Run the test
go run ./cmd/test-auth
```

## What the Test Does

The authentication test will:

1. **Create a PowerSchool client** with your credentials
2. **Launch a browser** (Chrome) to perform authentication
3. **Attempt to log in** using your username and password
4. **Extract session cookies** if login succeeds
5. **Try to fetch student data** to verify the session works

## Expected Outcomes

### Success Case

```
=================================================================
PowerSchool Authentication Test
=================================================================
URL: https://ps.example.org
Username: your_username
Password: y***d
...

=================================================================
AUTHENTICATION SUCCESSFUL!
=================================================================
Duration: 5.2s

Session Information:
  Expires at: 2025-10-24T18:30:00Z
  Time until expiry: 4h0m
  Number of cookies: 3

=================================================================
Testing data retrieval...
=================================================================

Fetching students...
Successfully fetched 1 student(s)
  1. Student Name (ID: 12345, Grade: 7)
```

### Failure Cases

#### Case 1: Wrong Login Form Selectors

If the HTML selectors don't match the reference instance's PowerSchool:

```
!!! AUTHENTICATION FAILED !!!
Error: authentication failed: failed to find login form elements
```

**Solution**: We'll need to inspect the actual HTML and update selectors in [internal/browser/browser.go](internal/browser/browser.go).

#### Case 2: Invalid Credentials

```
!!! AUTHENTICATION FAILED !!!
Error: authentication failed: login failed: invalid credentials
```

**Solution**: Verify your 1Password paths are correct.

#### Case 3: Unexpected Login Flow

PowerSchool might have:
- SSO redirect
- CAPTCHA
- Two-factor authentication
- District-specific login page

**Solution**: We'll need to adapt the authentication logic.

## Debugging

### Enable Debug Logging

```bash
./scripts/test-with-1password.sh --debug
```

This will show:
- All HTTP requests and responses
- HTML content (truncated)
- Browser automation steps
- Cookie details

### Run with Visible Browser

**IMPORTANT**: For first-time testing, always run without `--headless`:

```bash
./scripts/test-with-1password.sh
```

This lets you see:
- What pages load
- Where the authentication fails
- Any error messages in the browser
- Whether form fields are filled correctly

### Save HTML for Analysis

```bash
./scripts/test-with-1password.sh --debug --save-html
```

(Note: This feature is not yet fully implemented, but debug mode will log HTML snippets)

## Next Steps After Successful Authentication

Once authentication works:

1. **Examine the student data** returned
2. **Navigate to grades page** in your browser to see URL patterns
3. **Update the parsing logic** in:
   - [student.go](student.go) - Student list parsing
   - [grades.go](grades.go) - Grade parsing
   - [assignments.go](assignments.go) - Assignment parsing

## Troubleshooting

### Issue: "go: command not found"

**Solution**: Ensure Go is installed and in your PATH

### Issue: "op: command not found"

**Solution**: Install 1Password CLI

### Issue: "chromedp: chrome not found"

**Solution**: Install Chrome or Chromium browser

### Issue: Browser immediately crashes

**Solution**: Try disabling sandbox mode:
```go
// Edit internal/browser/browser.go
// Change NoSandbox default to true
```

### Issue: "Session cookies not found"

This likely means the login succeeded but we're looking in the wrong place for the session.

**Solution**: Run with visible browser and debug mode, watch what happens after login.

## Safety Notes

- ✅ **All operations are READ-ONLY**
- ✅ **No data modification** occurs
- ✅ **Credentials never logged** (even in debug mode, passwords are redacted)
- ✅ **Session files are gitignored**
- ⚠️ **Running tests uses your real account** (parent account)
- ⚠️ **Browser automation may trigger** security alerts

## Questions to Answer During Testing

As you test, please note:

1. **Did authentication succeed?**
2. **What is the actual login URL?** (check browser)
3. **What are the actual form field names?** (inspect with browser dev tools)
4. **What happens after login?** (redirect URL, dashboard page)
5. **Can you manually navigate to grades/assignments?** (note URLs)
6. **What does the student selector look like?** (if you have multiple students)

---

**Ready to test?** Run:

```bash
./scripts/test-with-1password.sh
```

And watch what happens!
