# Session Persistence Example

This example demonstrates how to persist and reuse PowerSchool sessions.

## Features

- Session export and import
- Automatic session reuse
- Fallback to credential authentication
- Session validation

## Usage

First run (creates new session):

```bash
export POWERSCHOOL_URL="https://powerschool.yourdistrict.org"
export POWERSCHOOL_USERNAME="your_username"
export POWERSCHOOL_PASSWORD="your_password"
go run main.go
```

Subsequent runs (reuses session):

```bash
export POWERSCHOOL_URL="https://powerschool.yourdistrict.org"
go run main.go
```

## How It Works

1. Checks for existing session file (`powerschool_session.json`)
2. If found, attempts to import and validate the session
3. If session is invalid or not found, authenticates with credentials
4. Saves the session to file for future use
5. Displays session expiration information

## Security Note

The session file contains authentication cookies. Protect this file:

```bash
chmod 600 powerschool_session.json
```

Never commit this file to version control!
