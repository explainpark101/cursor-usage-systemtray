"""Unofficial Cursor dashboard usage / billing client."""

from .client import UnauthorizedError, fetch_snapshot
from .cookies import parse_cookie_input
from .format import format_local_time, format_remaining, format_usd
from .types import PlanUsage, Snapshot

__all__ = [
    "PlanUsage",
    "Snapshot",
    "UnauthorizedError",
    "fetch_snapshot",
    "parse_cookie_input",
    "format_usd",
    "format_remaining",
    "format_local_time",
]
__version__ = "0.1.0"
