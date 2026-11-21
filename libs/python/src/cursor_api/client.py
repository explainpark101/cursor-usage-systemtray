from __future__ import annotations

import json
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from typing import Any, Optional

from .types import PlanUsage, Snapshot

USAGE_SUMMARY_URL = "https://cursor.com/api/usage-summary"
PERIOD_USAGE_URL = "https://cursor.com/api/dashboard/get-current-period-usage"
PLAN_INFO_URL = "https://cursor.com/api/dashboard/get-plan-info"
DEFAULT_TIMEOUT = 8.0
_MAX_BODY = 256 * 1024
_UA = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)


class UnauthorizedError(Exception):
    """Raised on HTTP 401/403 from Cursor APIs."""


def fetch_snapshot(cookie: str, *, timeout: float = DEFAULT_TIMEOUT) -> Snapshot:
    if not cookie:
        raise ValueError("no cookie configured")

    def get_summary() -> dict[str, Any]:
        return _request("GET", USAGE_SUMMARY_URL, cookie, None, timeout)

    def get_period() -> dict[str, Any]:
        return _request("POST", PERIOD_USAGE_URL, cookie, b"{}", timeout)

    def get_plan() -> dict[str, Any]:
        return _request("POST", PLAN_INFO_URL, cookie, b"{}", timeout)

    with ThreadPoolExecutor(max_workers=3) as pool:
        f_sum = pool.submit(get_summary)
        f_per = pool.submit(get_period)
        f_plan = pool.submit(get_plan)
        errors = []
        summary = period = plan = None
        try:
            summary = f_sum.result()
        except UnauthorizedError:
            raise
        except Exception as e:
            errors.append(e)
        try:
            period = f_per.result()
        except UnauthorizedError:
            raise
        except Exception:
            period = None
        try:
            plan = f_plan.result()
        except UnauthorizedError:
            raise
        except Exception:
            plan = None

    if summary is None:
        raise errors[0] if errors else RuntimeError("empty usage-summary")
    return _build_snapshot(summary, period, plan)


def _request(
    method: str, url: str, cookie: str, body: Optional[bytes], timeout: float
) -> dict[str, Any]:
    headers = {
        "Accept": "application/json, text/plain, */*",
        "Origin": "https://cursor.com",
        "Referer": "https://cursor.com/dashboard",
        "User-Agent": _UA,
        "Cookie": cookie,
    }
    if body is not None:
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read(_MAX_BODY)
            status = resp.status
    except urllib.error.HTTPError as e:
        raw = e.read(_MAX_BODY)
        status = e.code
        if status in (401, 403):
            msg = raw.decode("utf-8", "replace")
            if "Invalid origin" in msg:
                raise RuntimeError(f"rejected by CSRF origin check: {msg[:120]}") from e
            raise UnauthorizedError(f"HTTP {status}") from e
        raise RuntimeError(f"HTTP {status}: {raw[:120]!r}") from e
    if status != 200:
        raise RuntimeError(f"HTTP {status}: {raw[:120]!r}")
    return json.loads(raw.decode("utf-8"))


def _parse_time(value: Any) -> Optional[datetime]:
    if value is None:
        return None
    if isinstance(value, (int, float)):
        return datetime.fromtimestamp(float(value) / 1000.0, tz=timezone.utc)
    if isinstance(value, str):
        s = value.strip()
        if not s:
            return None
        try:
            if s.endswith("Z"):
                s = s[:-1] + "+00:00"
            return datetime.fromisoformat(s)
        except ValueError:
            pass
        try:
            return datetime.fromtimestamp(float(s) / 1000.0, tz=timezone.utc)
        except ValueError:
            return None
    return None


def _first_nonempty(*vals: str) -> str:
    for v in vals:
        if v and v.strip():
            return v
    return ""


def _build_snapshot(
    summary: dict[str, Any],
    period: Optional[dict[str, Any]],
    plan: Optional[dict[str, Any]],
) -> Snapshot:
    s = Snapshot(
        membership_type=summary.get("membershipType") or "",
        display_message=_first_nonempty(
            (period or {}).get("displayMessage") or "",
            (period or {}).get("autoModelSelectedDisplayMessage") or "",
            summary.get("autoModelSelectedDisplayMessage") or "",
        ),
        billing_cycle_start=_parse_time(summary.get("billingCycleStart")),
        billing_cycle_end=_parse_time(summary.get("billingCycleEnd")),
    )

    plan_info = (plan or {}).get("planInfo") or {}
    if plan_info:
        s.plan_name = plan_info.get("planName") or ""
        s.plan_price = plan_info.get("price") or ""
        renew = _parse_time(plan_info.get("renewsAt")) or _parse_time(
            plan_info.get("billingCycleEnd")
        )
        if renew is not None:
            s.billing_cycle_end = renew

    if period:
        if s.billing_cycle_start is None:
            s.billing_cycle_start = _parse_time(period.get("billingCycleStart"))
        if s.billing_cycle_end is None:
            s.billing_cycle_end = _parse_time(period.get("billingCycleEnd"))
        pu = period.get("planUsage") or {}
        if pu:
            s.total_spend_cents = int(pu.get("totalSpend") or 0)
            s.included_spend_cents = int(pu.get("includedSpend") or 0)
            s.bonus_spend_cents = int(pu.get("bonusSpend") or 0)
            s.plan_limit_cents = int(pu.get("limit") or 0)
            s.plan_usage = PlanUsage(
                total_percent_used=float(pu.get("totalPercentUsed") or 0),
                auto_percent_used=float(pu.get("autoPercentUsed") or 0),
                api_percent_used=float(pu.get("apiPercentUsed") or 0),
            )
        slu = period.get("spendLimitUsage") or {}
        if "individualUsed" in slu and slu["individualUsed"] is not None:
            s.on_demand_used_cents = int(slu["individualUsed"])
        if slu.get("individualLimit") is not None:
            s.on_demand_limit_cents = int(slu["individualLimit"])
        if slu.get("individualRemaining") is not None:
            s.on_demand_remaining_cents = int(slu["individualRemaining"])

    iu = summary.get("individualUsage") or {}
    p = iu.get("plan") or {}
    if p:
        s.included_enabled = bool(p.get("enabled"))
        s.included_used_cents = int(p.get("used") or 0)
        s.included_limit_cents = int(p.get("limit") or 0)
        s.included_remaining_cents = int(p.get("remaining") or 0)
        bd = p.get("breakdown") or {}
        if bd:
            s.included_bonus_cents = int(bd.get("bonus") or 0)
            s.included_breakdown_total = int(bd.get("total") or 0)
            if not s.included_spend_cents:
                s.included_spend_cents = int(bd.get("included") or 0)
            if not s.bonus_spend_cents:
                s.bonus_spend_cents = int(bd.get("bonus") or 0)
            if not s.total_spend_cents:
                s.total_spend_cents = int(bd.get("total") or 0)
        if not s.plan_limit_cents:
            s.plan_limit_cents = int(p.get("limit") or 0)
        s.plan_usage = PlanUsage(
            total_percent_used=float(p.get("totalPercentUsed") or 0),
            auto_percent_used=float(p.get("autoPercentUsed") or 0),
            api_percent_used=float(p.get("apiPercentUsed") or 0),
        )
    od = iu.get("onDemand") or {}
    if od:
        s.on_demand_enabled = bool(od.get("enabled"))
        s.on_demand_used_cents = int(od.get("used") or 0)
        s.on_demand_limit_cents = (
            int(od["limit"]) if od.get("limit") is not None else None
        )
        s.on_demand_remaining_cents = (
            int(od["remaining"]) if od.get("remaining") is not None else None
        )
    return s
