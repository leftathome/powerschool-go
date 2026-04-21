# Basic Example

This example demonstrates basic usage of the powerschool-go library.

## Features

- Authentication with credentials
- Fetching student information
- Retrieving grades and GPA
- Getting pending assignments

## Usage

Set environment variables:

```bash
export POWERSCHOOL_URL="https://powerschool.yourdistrict.org"
export POWERSCHOOL_USERNAME="your_username"
export POWERSCHOOL_PASSWORD="your_password"
```

Run the example:

```bash
go run main.go
```

## Output

The example will display:
- Student name and details
- Current grades for all courses
- GPA (if available)
- Pending assignments with due dates
