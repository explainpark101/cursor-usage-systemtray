# Cursor API libraries

Unofficial clients for Cursor dashboard usage / billing data, extracted from
the cursor-tray reverse-engineering work.

All four libraries talk to the same undocumented endpoints with a browser
session cookie (`WorkosCursorSessionToken`):

| Endpoint | Method |
|---|---|
| `/api/usage-summary` | GET |
| `/api/dashboard/get-current-period-usage` | POST `{}` |
| `/api/dashboard/get-plan-info` | POST `{}` |

Returned snapshot covers plan name/price, renewal countdown, spending (cents),
included usage, and on-demand usage.

| Language | Path | Package |
|---|---|---|
| Go | [`libs/go`](go/) | `cursor-api` (`cursorapi`) |
| Python | [`libs/python`](python/) | `cursor-api` (`cursor_api`) |
| TypeScript | [`libs/typescript`](typescript/) | `@cursor-tray/cursor-api` |
| Rust | [`libs/rust`](rust/) | `cursor-api` |

## Auth

Pass either:

- a `Cookie` header value, or
- a Netscape / curl `cookies.txt` file (parsed by each library), or
- a bare `WorkosCursorSessionToken` value

Always send `Origin: https://cursor.com` (handled by the clients) or Cursor
returns CSRF `403 Invalid origin`.

## Disclaimer

These APIs are undocumented and may change without notice. Session cookies are
secrets — do not commit them.
