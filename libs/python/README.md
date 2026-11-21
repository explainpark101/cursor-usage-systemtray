# cursor-api (Python)

Unofficial Python client for Cursor dashboard usage / billing data.
Stdlib only (`urllib`). Requires a session cookie (`WorkosCursorSessionToken`).

## Install

```bash
pip install -e libs/python
```

## Usage

```python
from cursor_api import parse_cookie_input, fetch_snapshot, format_usd, format_remaining

cookie = parse_cookie_input(open("cookies.txt").read())
snap = fetch_snapshot(cookie)
print(snap.plan_name, format_remaining(snap.remaining()))
print(format_usd(snap.total_spend_cents))
```

## CLI-ish example

```bash
CURSOR_COOKIE='WorkosCursorSessionToken=...' python -c "from cursor_api import *; import os; s=fetch_snapshot(parse_cookie_input(os.environ['CURSOR_COOKIE'])); print(s)"
```
