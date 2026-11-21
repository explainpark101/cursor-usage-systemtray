use std::time::{SystemTime, UNIX_EPOCH};

use crate::client::Error;

/// Parse Cookie header or Netscape cookies.txt into a Cookie header string.
pub fn parse_cookie_input(raw: &str) -> Result<String, Error> {
    let raw = raw.trim();
    if raw.is_empty() {
        return Err(Error::Cookie("cookie input is empty".into()));
    }
    if looks_like_netscape(raw) {
        let header = parse_netscape(raw)?;
        if header.is_empty() {
            return Err(Error::Cookie(
                "no cursor.com cookies found in cookie file".into(),
            ));
        }
        return Ok(header);
    }
    let header = trim_header(raw);
    if header.is_empty() {
        return Err(Error::Cookie("cookie value is empty".into()));
    }
    Ok(normalize_header(&header))
}

fn normalize_header(header: &str) -> String {
    let mut header = header.to_string();
    if header.to_ascii_lowercase().starts_with("cookie:") {
        header = header["cookie:".len()..].trim().to_string();
    }
    if !header.contains('=') && !header.contains(';') && !header.is_empty() {
        return format!("WorkosCursorSessionToken={header}");
    }
    header
}

fn looks_like_netscape(raw: &str) -> bool {
    let lower = raw.to_ascii_lowercase();
    if lower.contains("netscape http cookie file")
        || lower.contains("# httponly_")
        || raw.contains("#HttpOnly_")
    {
        return true;
    }
    for line in raw.lines() {
        let t = line.trim();
        if t.is_empty() || t.starts_with('#') {
            continue;
        }
        if line.matches('\t').count() >= 6 {
            return true;
        }
        let fields: Vec<_> = t.split_whitespace().collect();
        if fields.len() >= 7 && (fields[1] == "TRUE" || fields[1] == "FALSE") {
            return true;
        }
    }
    false
}

fn is_cursor_domain(domain: &str) -> bool {
    let d = domain.trim_start_matches('.').to_ascii_lowercase();
    d == "cursor.com" || d.ends_with(".cursor.com")
}

fn now_unix() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

fn parse_netscape(raw: &str) -> Result<String, Error> {
    let now = now_unix();
    let mut parts: Vec<String> = Vec::new();
    let mut seen = std::collections::HashSet::new();
    let mut parsed = 0usize;

    for line in raw.lines() {
        let line = line.trim_end_matches('\r');
        if line.is_empty() {
            continue;
        }
        let line = if let Some(rest) = line.strip_prefix("#HttpOnly_") {
            rest
        } else if line.trim_start().starts_with('#') {
            continue;
        } else {
            line
        };

        let mut fields: Vec<&str> = line.split('\t').collect();
        if fields.len() < 7 {
            let ws: Vec<_> = line.split_whitespace().collect();
            if ws.len() < 7 {
                continue;
            }
            fields = ws;
        }

        let domain = fields[0].trim();
        let name = fields[5].trim();
        let value = fields[6];
        if domain.is_empty() || name.is_empty() || !is_cursor_domain(domain) {
            continue;
        }
        let exp_str = fields[4].trim();
        if !exp_str.is_empty() && exp_str != "0" {
            if let Ok(exp) = exp_str.parse::<i64>() {
                if exp > 0 && exp < now {
                    continue;
                }
            }
        }
        if seen.contains(name) {
            parts.retain(|p| !p.starts_with(&format!("{name}=")));
        }
        seen.insert(name.to_string());
        parts.push(format!("{name}={value}"));
        parsed += 1;
    }

    if parsed == 0 && parts.is_empty() {
        if has_any_cookie_line(raw) {
            return Ok(String::new());
        }
        return Err(Error::Cookie(
            "no cookie lines found (expected Netscape cookies.txt)".into(),
        ));
    }
    Ok(parts.join("; "))
}

fn has_any_cookie_line(raw: &str) -> bool {
    for line in raw.lines() {
        if line.starts_with("#HttpOnly_") {
            return true;
        }
        let t = line.trim();
        if t.is_empty() || t.starts_with('#') {
            continue;
        }
        if line.matches('\t').count() >= 6 || t.split_whitespace().count() >= 7 {
            return true;
        }
    }
    false
}

fn trim_header(s: &str) -> String {
    s.chars()
        .map(|c| match c {
            '\n' | '\r' | '\t' => ' ',
            o => o,
        })
        .collect::<String>()
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
}
