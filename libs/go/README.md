# cursor-api (Go)

Unofficial Go client for Cursor dashboard usage / billing data
(`usage-summary`, `get-current-period-usage`, `get-plan-info`).

Requires a browser session cookie (`WorkosCursorSessionToken`). APIs are
undocumented and may change without notice.

## Install

```bash
go get cursor-api@latest
# or, from this repo:
go mod edit -require=cursor-api@v0.0.0
go mod edit -replace=cursor-api=./libs/go
```

## Usage

```go
cookie, err := cursorapi.ParseCookieInput(raw) // Cookie header or Netscape cookies.txt
snap, err := cursorapi.Fetch(ctx, cookie)
fmt.Println(snap.PlanName, cursorapi.FormatRemaining(snap.Remaining(time.Now())))
```

## Example

```bash
cd libs/go
CURSOR_COOKIE='WorkosCursorSessionToken=...' go run ./examples/fetch
# or
go run ./examples/fetch /path/to/cookies.txt
```

## License

Same as the parent cursor-tray project.
