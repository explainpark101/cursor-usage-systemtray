from __future__ import annotations

import time


def parse_cookie_input(raw: str) -> str:
    """Parse Cookie header or Netscape cookies.txt into a Cookie header string."""
    raw = raw.strip()
    if not raw:
        raise ValueError("cookie input is empty")
    if _looks_like_netscape(raw):
        header = _parse_netscape(raw)
        if not header:
            raise ValueError("no cursor.com cookies found in cookie file")
        return header
    header = _trim_header(raw)
    if not header:
        raise ValueError("cookie value is empty")
    return _normalize_header(header)


def _normalize_header(header: str) -> str:
    lower = header.lower()
    if lower.startswith("cookie:"):
        header = header[len("cookie:") :].strip()
    if "=" not in header and ";" not in header and header:
        return f"WorkosCursorSessionToken={header}"
    return header


def _looks_like_netscape(raw: str) -> bool:
    lower = raw.lower()
    if (
        "netscape http cookie file" in lower
        or "# httponly_" in lower
        or "#HttpOnly_" in raw
    ):
        return True
    for line in raw.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if line.count("\t") >= 6:
            return True
        fields = line.split()
        if len(fields) >= 7 and fields[1] in ("TRUE", "FALSE"):
            return True
    return False


def _is_cursor_domain(domain: str) -> bool:
    d = domain.lstrip(".").lower()
    return d == "cursor.com" or d.endswith(".cursor.com")


def _parse_netscape(raw: str) -> str:
    now = int(time.time())
    parts: list[str] = []
    seen: dict[str, int] = {}
    parsed = 0

    for line in raw.splitlines():
        line = line.rstrip("\r")
        if not line:
            continue
        if line.startswith("#HttpOnly_"):
            line = line[len("#HttpOnly_") :]
        elif line.lstrip().startswith("#"):
            continue

        fields = line.split("\t")
        if len(fields) < 7:
            fields = line.split()
            if len(fields) < 7:
                continue
            if len(fields) > 7:
                fields = fields[:6] + [" ".join(fields[6:])]
        elif len(fields) > 7:
            fields = fields[:6] + ["\t".join(fields[6:])]

        domain, name, value = fields[0].strip(), fields[5].strip(), fields[6]
        if not domain or not name or not _is_cursor_domain(domain):
            continue
        exp_str = fields[4].strip()
        if exp_str and exp_str != "0":
            try:
                exp = int(exp_str)
                if exp > 0 and exp < now:
                    continue
            except ValueError:
                pass
        if name in seen:
            parts = [p for p in parts if not p.startswith(name + "=")]
        seen[name] = 1
        parts.append(f"{name}={value}")
        parsed += 1

    if parsed == 0 and not parts:
        if _has_any_cookie_line(raw):
            return ""
        raise ValueError("no cookie lines found (expected Netscape cookies.txt)")
    return "; ".join(parts)


def _has_any_cookie_line(raw: str) -> bool:
    for line in raw.splitlines():
        line = line.rstrip("\r")
        if line.startswith("#HttpOnly_"):
            return True
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        if line.count("\t") >= 6 or len(line.split()) >= 7:
            return True
    return False


def _trim_header(s: str) -> str:
    s = s.replace("\n", " ").replace("\r", " ").replace("\t", " ")
    while "  " in s:
        s = s.replace("  ", " ")
    return s.strip()
