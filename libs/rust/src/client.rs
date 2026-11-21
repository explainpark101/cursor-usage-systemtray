use serde_json::Value;
use std::io::Write;
use std::process::{Command, Stdio};
use crate::types::{PlanUsage, Snapshot};

const USAGE_SUMMARY_URL: &str = "https://cursor.com/api/usage-summary";
const PERIOD_USAGE_URL: &str = "https://cursor.com/api/dashboard/get-current-period-usage";
const PLAN_INFO_URL: &str = "https://cursor.com/api/dashboard/get-plan-info";
const UA: &str = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36";

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("unauthorized (HTTP 401/403)")]
    Unauthorized,
    #[error("cookie: {0}")]
    Cookie(String),
    #[error("http: {0}")]
    Http(String),
    #[error(transparent)]
    Json(#[from] serde_json::Error),
    #[error("transport: {0}")]
    Transport(String),
}

/// Marker type alias for unauthorized failures.
pub type Unauthorized = Error;

/// Fetch a combined dashboard snapshot using a Cookie header value.
///
/// Uses the system `curl` binary for HTTPS so this crate builds without a
/// native TLS / C toolchain dependency.
pub fn fetch_snapshot(cookie: &str) -> Result<Snapshot, Error> {
    if cookie.is_empty() {
        return Err(Error::Cookie("no cookie configured".into()));
    }

    let summary = get_json("GET", USAGE_SUMMARY_URL, cookie, None);
    let period = get_json("POST", PERIOD_USAGE_URL, cookie, Some("{}"));
    let plan = get_json("POST", PLAN_INFO_URL, cookie, Some("{}"));

    for r in [&summary, &period, &plan] {
        if let Err(Error::Unauthorized) = r {
            return Err(Error::Unauthorized);
        }
    }
    let summary = summary?;
    let period = period.ok();
    let plan = plan.ok();
    Ok(build_snapshot(&summary, period.as_ref(), plan.as_ref()))
}

fn get_json(method: &str, url: &str, cookie: &str, body: Option<&str>) -> Result<Value, Error> {
    let mut cmd = Command::new("curl");
    cmd.arg("-sS")
        .arg("-X")
        .arg(method)
        .arg(url)
        .arg("-H")
        .arg("Accept: application/json, text/plain, */*")
        .arg("-H")
        .arg("Origin: https://cursor.com")
        .arg("-H")
        .arg("Referer: https://cursor.com/dashboard")
        .arg("-H")
        .arg(format!("User-Agent: {UA}"))
        .arg("-H")
        .arg(format!("Cookie: {cookie}"))
        .arg("-w")
        .arg("\n__HTTP_STATUS__:%{http_code}")
        .arg("--max-time")
        .arg("8")
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());

    if body.is_some() {
        cmd.arg("-H").arg("Content-Type: application/json");
        cmd.arg("--data-binary").arg("@-");
        cmd.stdin(Stdio::piped());
    }

    let mut child = cmd
        .spawn()
        .map_err(|e| Error::Transport(format!("failed to spawn curl: {e}")))?;

    if let Some(b) = body {
        if let Some(mut stdin) = child.stdin.take() {
            stdin
                .write_all(b.as_bytes())
                .map_err(|e| Error::Transport(e.to_string()))?;
        }
    }

    let output = child
        .wait_with_output()
        .map_err(|e| Error::Transport(e.to_string()))?;
    if !output.status.success() && output.stdout.is_empty() {
        let err = String::from_utf8_lossy(&output.stderr);
        return Err(Error::Transport(format!("curl failed: {err}")));
    }

    let raw = String::from_utf8_lossy(&output.stdout);
    let (body_text, status) = split_status(&raw)?;
    if status == 401 || status == 403 {
        if body_text.contains("Invalid origin") {
            return Err(Error::Http(format!(
                "rejected by CSRF origin check: {}",
                &body_text[..body_text.len().min(120)]
            )));
        }
        return Err(Error::Unauthorized);
    }
    if status != 200 {
        return Err(Error::Http(format!(
            "HTTP {}: {}",
            status,
            &body_text[..body_text.len().min(120)]
        )));
    }
    Ok(serde_json::from_str(body_text)?)
}

fn split_status(raw: &str) -> Result<(&str, u16), Error> {
    let marker = "\n__HTTP_STATUS__:";
    let idx = raw
        .rfind(marker)
        .ok_or_else(|| Error::Transport("curl status marker missing".into()))?;
    let body = &raw[..idx];
    let code_str = raw[idx + marker.len()..].trim();
    let status: u16 = code_str
        .parse()
        .map_err(|_| Error::Transport(format!("bad status: {code_str}")))?;
    Ok((body, status))
}

fn parse_time(v: &Value) -> Option<i64> {
    match v {
        Value::Null => None,
        Value::Number(n) => n.as_f64().map(|f| f as i64),
        Value::String(s) => {
            let s = s.trim();
            if s.is_empty() {
                return None;
            }
            if let Ok(n) = s.parse::<f64>() {
                return Some(n as i64);
            }
            time_parse_rfc3339(s).ok()
        }
        _ => None,
    }
}

fn time_parse_rfc3339(s: &str) -> Result<i64, ()> {
    let s = s.replace('Z', "+00:00");
    let (date, rest) = s.split_once('T').ok_or(())?;
    let mut date_parts = date.split('-');
    let y: i64 = date_parts.next().ok_or(())?.parse().map_err(|_| ())?;
    let m: i64 = date_parts.next().ok_or(())?.parse().map_err(|_| ())?;
    let d: i64 = date_parts.next().ok_or(())?.parse().map_err(|_| ())?;

    let (time_part, offset_part) = if let Some(i) = rest.rfind('+').or_else(|| {
        rest.char_indices()
            .rev()
            .find(|&(i, c)| c == '-' && i > 0)
            .map(|(i, _)| i)
    }) {
        (&rest[..i], &rest[i..])
    } else {
        (rest, "+00:00")
    };

    let mut t = time_part.split(':');
    let hh: i64 = t.next().ok_or(())?.parse().map_err(|_| ())?;
    let mm: i64 = t.next().ok_or(())?.parse().map_err(|_| ())?;
    let ss_raw = t.next().unwrap_or("0");
    let ss: i64 = ss_raw
        .split('.')
        .next()
        .unwrap_or("0")
        .parse()
        .map_err(|_| ())?;

    let days = days_from_civil(y, m, d);
    let mut epoch = days * 86400 + hh * 3600 + mm * 60 + ss;

    let sign = if offset_part.starts_with('-') { -1 } else { 1 };
    let off = offset_part.trim_start_matches(['+', '-']);
    let mut op = off.split(':');
    let oh: i64 = op.next().unwrap_or("0").parse().unwrap_or(0);
    let om: i64 = op.next().unwrap_or("0").parse().unwrap_or(0);
    epoch -= sign * (oh * 3600 + om * 60);

    Ok(epoch * 1000)
}

fn days_from_civil(y: i64, m: i64, d: i64) -> i64 {
    let y = if m <= 2 { y - 1 } else { y };
    let era = if y >= 0 { y } else { y - 399 } / 400;
    let yoe = (y - era * 400) as u64;
    let mp = if m > 2 { m - 3 } else { m + 9 };
    let doy = (153 * mp + 2) / 5 + d - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy as u64;
    (era * 146097 + doe as i64) - 719468
}

fn first_nonempty(vals: &[&str]) -> String {
    for v in vals {
        if !v.trim().is_empty() {
            return (*v).to_string();
        }
    }
    String::new()
}

fn i64_field(obj: &Value, key: &str) -> i64 {
    obj.get(key)
        .and_then(|v| v.as_i64().or_else(|| v.as_f64().map(|f| f as i64)))
        .unwrap_or(0)
}

fn f64_field(obj: &Value, key: &str) -> f64 {
    obj.get(key).and_then(|v| v.as_f64()).unwrap_or(0.0)
}

fn opt_i64(obj: &Value, key: &str) -> Option<i64> {
    obj.get(key).and_then(|v| {
        if v.is_null() {
            None
        } else {
            v.as_i64().or_else(|| v.as_f64().map(|f| f as i64))
        }
    })
}

pub(crate) fn build_snapshot(
    summary: &Value,
    period: Option<&Value>,
    plan: Option<&Value>,
) -> Snapshot {
    let mut s = Snapshot {
        membership_type: summary
            .get("membershipType")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string(),
        display_message: first_nonempty(&[
            period
                .and_then(|p| p.get("displayMessage"))
                .and_then(|v| v.as_str())
                .unwrap_or(""),
            period
                .and_then(|p| p.get("autoModelSelectedDisplayMessage"))
                .and_then(|v| v.as_str())
                .unwrap_or(""),
            summary
                .get("autoModelSelectedDisplayMessage")
                .and_then(|v| v.as_str())
                .unwrap_or(""),
        ]),
        billing_cycle_start_ms: summary.get("billingCycleStart").and_then(parse_time),
        billing_cycle_end_ms: summary.get("billingCycleEnd").and_then(parse_time),
        ..Default::default()
    };

    if let Some(plan_info) = plan.and_then(|p| p.get("planInfo")) {
        s.plan_name = plan_info
            .get("planName")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        s.plan_price = plan_info
            .get("price")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        if let Some(t) = plan_info
            .get("renewsAt")
            .and_then(parse_time)
            .or_else(|| plan_info.get("billingCycleEnd").and_then(parse_time))
        {
            s.billing_cycle_end_ms = Some(t);
        }
    }

    if let Some(period) = period {
        if s.billing_cycle_start_ms.is_none() {
            s.billing_cycle_start_ms = period.get("billingCycleStart").and_then(parse_time);
        }
        if s.billing_cycle_end_ms.is_none() {
            s.billing_cycle_end_ms = period.get("billingCycleEnd").and_then(parse_time);
        }
        if let Some(pu) = period.get("planUsage") {
            s.total_spend_cents = i64_field(pu, "totalSpend");
            s.included_spend_cents = i64_field(pu, "includedSpend");
            s.bonus_spend_cents = i64_field(pu, "bonusSpend");
            s.plan_limit_cents = i64_field(pu, "limit");
            s.plan_usage = PlanUsage {
                total_percent_used: f64_field(pu, "totalPercentUsed"),
                auto_percent_used: f64_field(pu, "autoPercentUsed"),
                api_percent_used: f64_field(pu, "apiPercentUsed"),
            };
        }
        if let Some(slu) = period.get("spendLimitUsage") {
            if let Some(v) = opt_i64(slu, "individualUsed") {
                s.on_demand_used_cents = v;
            }
            s.on_demand_limit_cents = opt_i64(slu, "individualLimit");
            s.on_demand_remaining_cents = opt_i64(slu, "individualRemaining");
        }
    }

    if let Some(iu) = summary.get("individualUsage") {
        if let Some(p) = iu.get("plan") {
            s.included_enabled = p.get("enabled").and_then(|v| v.as_bool()).unwrap_or(false);
            s.included_used_cents = i64_field(p, "used");
            s.included_limit_cents = i64_field(p, "limit");
            s.included_remaining_cents = i64_field(p, "remaining");
            if let Some(bd) = p.get("breakdown") {
                s.included_bonus_cents = i64_field(bd, "bonus");
                s.included_breakdown_total = i64_field(bd, "total");
                if s.included_spend_cents == 0 {
                    s.included_spend_cents = i64_field(bd, "included");
                }
                if s.bonus_spend_cents == 0 {
                    s.bonus_spend_cents = i64_field(bd, "bonus");
                }
                if s.total_spend_cents == 0 {
                    s.total_spend_cents = i64_field(bd, "total");
                }
            }
            if s.plan_limit_cents == 0 {
                s.plan_limit_cents = i64_field(p, "limit");
            }
            s.plan_usage = PlanUsage {
                total_percent_used: f64_field(p, "totalPercentUsed"),
                auto_percent_used: f64_field(p, "autoPercentUsed"),
                api_percent_used: f64_field(p, "apiPercentUsed"),
            };
        }
        if let Some(od) = iu.get("onDemand") {
            s.on_demand_enabled = od.get("enabled").and_then(|v| v.as_bool()).unwrap_or(false);
            s.on_demand_used_cents = i64_field(od, "used");
            s.on_demand_limit_cents = opt_i64(od, "limit");
            s.on_demand_remaining_cents = opt_i64(od, "remaining");
        }
    }

    s
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn merges_summary_period_plan() {
        let summary = json!({
            "billingCycleStart": "2026-10-21T16:15:29.000Z",
            "billingCycleEnd": "2026-11-21T16:15:29.000Z",
            "membershipType": "pro_plus",
            "individualUsage": {
                "plan": {
                    "enabled": true,
                    "used": 7000,
                    "limit": 7000,
                    "remaining": 0,
                    "breakdown": {"included": 7000, "bonus": 75693, "total": 82693},
                    "autoPercentUsed": 68.9,
                    "apiPercentUsed": 0,
                    "totalPercentUsed": 67.3
                },
                "onDemand": {"enabled": false, "used": 0, "limit": null, "remaining": null}
            }
        });
        let period = json!({
            "planUsage": {
                "totalSpend": 82693,
                "includedSpend": 7000,
                "bonusSpend": 75693,
                "limit": 7000
            },
            "displayMessage": "hit"
        });
        let plan = json!({
            "planInfo": {
                "planName": "Pro+",
                "price": "$60/mo",
                "renewsAt": "1795277729000"
            }
        });
        let s = build_snapshot(&summary, Some(&period), Some(&plan));
        assert_eq!(s.plan_name, "Pro+");
        assert_eq!(s.total_spend_cents, 82693);
        assert!(!s.on_demand_enabled);
        assert_eq!(s.billing_cycle_end_ms, Some(1795277729000));
    }
}
