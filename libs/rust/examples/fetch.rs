use cursor_api::{
    fetch_snapshot, format_remaining, format_usd, parse_cookie_input,
};
use std::env;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let raw = env::var("CURSOR_COOKIE").or_else(|_| {
        let path = env::args().nth(1).ok_or_else(|| {
            std::io::Error::new(
                std::io::ErrorKind::InvalidInput,
                "set CURSOR_COOKIE or pass cookies.txt path",
            )
        })?;
        std::fs::read_to_string(path)
    })?;

    let cookie = parse_cookie_input(&raw)?;
    let snap = fetch_snapshot(&cookie)?;

    println!("plan={} ({})", snap.plan_name, snap.plan_price);
    println!("remain={}", format_remaining(snap.remaining_ms()));
    println!(
        "total={:.1}% auto={:.1}% api={:.1}%",
        snap.plan_usage.total_percent_used,
        snap.plan_usage.auto_percent_used,
        snap.plan_usage.api_percent_used
    );
    println!(
        "spending={} (included {} + bonus {})",
        format_usd(snap.total_spend_cents),
        format_usd(snap.included_spend_cents),
        format_usd(snap.bonus_spend_cents)
    );
    println!(
        "included={} / {} remaining={}",
        format_usd(snap.included_used_cents),
        format_usd(snap.included_limit_cents),
        format_usd(snap.included_remaining_cents)
    );
    println!(
        "onDemand enabled={} used={}",
        snap.on_demand_enabled,
        format_usd(snap.on_demand_used_cents)
    );
    Ok(())
}
