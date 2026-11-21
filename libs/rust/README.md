# cursor-api (Rust)

Unofficial Rust client for Cursor dashboard usage / billing data.
Uses the system `curl` binary for HTTPS (no C TLS toolchain required).
Requires a session cookie (`WorkosCursorSessionToken`).

## Usage

```toml
# Cargo.toml
cursor-api = { path = "libs/rust" }
```

```rust
use cursor_api::{parse_cookie_input, fetch_snapshot, format_usd, format_remaining};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let cookie = parse_cookie_input(&std::env::var("CURSOR_COOKIE")?)?;
    let snap = fetch_snapshot(&cookie)?;
    println!("{} {}", snap.plan_name, format_remaining(snap.remaining_ms()));
    println!("{}", format_usd(snap.total_spend_cents));
    Ok(())
}
```

## Example

```bash
cd libs/rust
CURSOR_COOKIE='WorkosCursorSessionToken=...' cargo run --example fetch
```
