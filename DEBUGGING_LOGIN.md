# Debugging Login Issues

This guide will help you identify and fix login form selector issues.

## What I Changed

### 1. Fixed Chrome Profile Prompts
Added flags to disable Chrome's sync/profile prompts:
- `--no-first-run`
- `--no-default-browser-check`
- `--disable-sync`
- `--disable-features=TranslateUI`
- `--disable-extensions`

### 2. Made Selectors More Flexible
The code now tries multiple selectors for each field:

**Username field** tries:
- `input[name="account"]`
- `input[name="username"]`
- `input[id="fieldAccount"]`
- `input[type="text"]`

**Password field** tries:
- `input[name="pw"]`
- `input[name="password"]`
- `input[id="fieldPassword"]`
- `input[type="password"]`

**Submit button** tries:
- `input[type="submit"]`
- `button[type="submit"]`
- `button.button-login`
- `input.button-login``
- `#btn-enter`
- `button:contains("Sign In")`
- `input[value*="Sign"]`
- If all fail, presses Enter on password field

### 3. Added Debug Output
When you run with `-debug`, you'll see:
- Page title and URL
- Login form HTML (first 500 chars)
- Which selector matched for each field
- Whether the submit button was clicked

## How to Test

Run with debug mode to see what's happening:

```powershell
# Fetch credentials from 1Password
$env:POWERSCHOOL_URL = "https://ps.example.org"
$env:POWERSCHOOL_USERNAME = op read "op://Private/your-powerschool-item/username"
$env:POWERSCHOOL_PASSWORD = op read "op://Private/your-powerschool-item/password"

# Run with debug (and visible browser)
go run ./cmd/test-auth -debug
```

## What to Look For

### In the Debug Output

Look for lines like:
```
[chromedp] Page loaded - Title: PowerSchool Sign In, URL: https://...
[chromedp] Login form HTML (first 500 chars):
<form ...>
  <input name="XXX" ...>  <- What is the actual name?
  ...
</form>
[chromedp] Found username field with selector: input[name="account"]
[chromedp] Found password field with selector: input[type="password"]
[chromedp] Found submit button with selector: input[type="submit"]
[chromedp] Successfully clicked submit button
```

### In the Browser

Watch for:
1. Does it fill in the username? ✓
2. Does it fill in the password? ✓
3. Does the submit button get clicked? ← **This is the problem**
4. Does the page navigate after clicking?

## Common Issues and Fixes

### Issue 1: Button Doesn't Get Clicked

**Symptom**: Fields are filled but nothing happens

**Debug**: Look at the form HTML output. Find the actual submit button element.

**Example fixes needed**:

If the button looks like:
```html
<button id="submit_button" class="btn">Sign In</button>
```

Add to selectors in [internal/browser/browser.go](internal/browser/browser.go:150):
```go
`#submit_button`,
```

If it's:
```html
<a href="#" onclick="submitForm()">Sign In</a>
```

This is a link, not a button! We'd need JavaScript execution:
```go
chromedp.Evaluate(`submitForm()`, nil)
```

### Issue 2: Wrong Form Fields

**Symptom**: `could not find username input field` error

**Solution**:
1. Look at the debug output for form HTML
2. Find the actual `name` or `id` of the input
3. Add it to the selectors list

### Issue 3: JavaScript-Based Login

**Symptom**: Form exists but clicking does nothing

**Solution**: The site might use JavaScript to submit. Look for:
- `onclick` handlers
- Form submission via AJAX
- Need to execute JavaScript instead of clicking

## Manual Browser Inspection

If debug output isn't enough, manually inspect in Chrome:

1. Open PowerSchool login in Chrome
2. Press F12 (DevTools)
3. Go to Elements tab
4. Find the login form
5. Look for:
   - `<form>` element
   - `<input>` field names/ids
   - `<button>` or `<input type="submit">` attributes
   - Any `onclick` or JavaScript

## Next Steps After You Test

Please run the test with `-debug` and share:

1. **The debug output** - especially the form HTML
2. **What you saw in the browser** - did it click? did it navigate?
3. **Any errors** - from the console or program

Then I can:
- Add the correct selectors
- Handle any JavaScript-based submission
- Fix any other issues discovered

## Quick Test Command

```powershell
# All in one
$env:POWERSCHOOL_URL = "https://ps.example.org"
$env:POWERSCHOOL_USERNAME = op read "op://Private/your-powerschool-item/username"
$env:POWERSCHOOL_PASSWORD = op read "op://Private/your-powerschool-item/password"
go run ./cmd/test-auth -debug 2>&1 | Tee-Object -FilePath debug-output.txt
```

This saves the debug output to `debug-output.txt` while showing it on screen.
