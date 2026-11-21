use std::time::{Duration, UNIX_EPOCH};

pub fn format_usd(cents: i64) -> String {
    let neg = cents < 0;
    let cents = cents.abs();
    let s = format!("${}.{:02}", cents / 100, cents % 100);
    if neg {
        format!("-{s}")
    } else {
        s
    }
}

pub fn format_remaining(ms: i64) -> String {
    let total_sec = (ms.max(0) / 1000) as i64;
    let days = total_sec / (60 * 60 * 24);
    let hours = (total_sec % (60 * 60 * 24)) / (60 * 60);
    let mins = (total_sec % (60 * 60)) / 60;
    let secs = total_sec % 60;
    format!("{days}d {hours}h {mins}m {secs}s")
}

pub fn format_remaining_duration(d: Duration) -> String {
    format_remaining(d.as_millis() as i64)
}

pub fn format_local_time(epoch_ms: Option<i64>) -> String {
    let Some(ms) = epoch_ms else {
        return "—".to_string();
    };
    let Some(t) = UNIX_EPOCH.checked_add(Duration::from_millis(ms as u64)) else {
        return "—".to_string();
    };
    // Local wall clock via chrono would be nicer; keep std-only formatting as UTC ISO-ish.
    match t.duration_since(UNIX_EPOCH) {
        Ok(d) => {
            let secs = d.as_secs();
            let days = secs / 86400;
            let rem = secs % 86400;
            let hours = rem / 3600;
            let mins = (rem % 3600) / 60;
            // Approximate civil date from days since epoch (UTC).
            let (y, m, day) = civil_from_days(days as i64);
            format!("{y:04}-{m:02}-{day:02} {hours:02}:{mins:02} UTC")
        }
        Err(_) => "—".to_string(),
    }
}

/// Howard Hinnant civil_from_days (UTC).
fn civil_from_days(z: i64) -> (i32, u32, u32) {
    let z = z + 719468;
    let era = if z >= 0 { z } else { z - 146096 } / 146097;
    let doe = (z - era * 146097) as u64;
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    let y = (yoe as i64) + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let y = if m <= 2 { y + 1 } else { y };
    (y as i32, m as u32, d as u32)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn usd_and_remaining() {
        assert_eq!(format_usd(7000), "$70.00");
        assert_eq!(format_usd(75693), "$756.93");
        assert_eq!(format_remaining(((26 * 60 + 5) * 60 + 7) * 1000), "1d 2h 5m 7s");
    }
}
