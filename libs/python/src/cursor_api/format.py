from __future__ import annotations

from datetime import datetime, timedelta, timezone
from typing import Optional


def format_usd(cents: int) -> str:
    neg = cents < 0
    cents = abs(cents)
    s = f"${cents // 100}.{cents % 100:02d}"
    return f"-{s}" if neg else s


def format_usd_optional(cents: Optional[int]) -> str:
    if cents is None:
        return "—"
    return format_usd(cents)


def format_remaining(delta: timedelta) -> str:
    total_sec = max(0, int(delta.total_seconds()))
    days = total_sec // (60 * 60 * 24)
    hours = (total_sec % (60 * 60 * 24)) // (60 * 60)
    mins = (total_sec % (60 * 60)) // 60
    secs = total_sec % 60
    return f"{days}d {hours}h {mins}m {secs}s"


def format_local_time(dt: Optional[datetime]) -> str:
    if dt is None:
        return "—"
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    return dt.astimezone().strftime("%Y-%m-%d %H:%M")
