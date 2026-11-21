from datetime import datetime, timedelta, timezone

from cursor_api.client import _build_snapshot
from cursor_api.format import format_remaining, format_usd


def test_build_snapshot_merges():
    summary = {
        "billingCycleStart": "2026-10-21T16:15:29.000Z",
        "billingCycleEnd": "2026-11-21T16:15:29.000Z",
        "membershipType": "pro_plus",
        "individualUsage": {
            "plan": {
                "enabled": True,
                "used": 7000,
                "limit": 7000,
                "remaining": 0,
                "breakdown": {"included": 7000, "bonus": 75693, "total": 82693},
                "autoPercentUsed": 68.9,
                "apiPercentUsed": 0,
                "totalPercentUsed": 67.3,
            },
            "onDemand": {"enabled": False, "used": 0, "limit": None, "remaining": None},
        },
    }
    period = {
        "planUsage": {
            "totalSpend": 82693,
            "includedSpend": 7000,
            "bonusSpend": 75693,
            "limit": 7000,
            "totalPercentUsed": 67.3,
            "autoPercentUsed": 68.9,
            "apiPercentUsed": 0,
        },
        "displayMessage": "You've hit your usage limit",
    }
    plan = {"planInfo": {"planName": "Pro+", "price": "$60/mo", "renewsAt": "1795277729000"}}
    s = _build_snapshot(summary, period, plan)
    assert s.plan_name == "Pro+"
    assert s.total_spend_cents == 82693
    assert not s.on_demand_enabled
    assert s.billing_cycle_end == datetime.fromtimestamp(
        1795277729000 / 1000, tz=timezone.utc
    )
    assert format_usd(7000) == "$70.00"
    assert format_remaining(timedelta(hours=26, minutes=5, seconds=7)) == "1d 2h 5m 7s"
