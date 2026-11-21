from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from typing import Optional


@dataclass
class PlanUsage:
    total_percent_used: float = 0.0
    auto_percent_used: float = 0.0
    api_percent_used: float = 0.0


@dataclass
class Snapshot:
    plan_usage: PlanUsage = field(default_factory=PlanUsage)
    membership_type: str = ""
    plan_name: str = ""
    plan_price: str = ""
    display_message: str = ""
    billing_cycle_start: Optional[datetime] = None
    billing_cycle_end: Optional[datetime] = None
    total_spend_cents: int = 0
    included_spend_cents: int = 0
    bonus_spend_cents: int = 0
    plan_limit_cents: int = 0
    included_enabled: bool = False
    included_used_cents: int = 0
    included_limit_cents: int = 0
    included_remaining_cents: int = 0
    included_bonus_cents: int = 0
    included_breakdown_total: int = 0
    on_demand_enabled: bool = False
    on_demand_used_cents: int = 0
    on_demand_limit_cents: Optional[int] = None
    on_demand_remaining_cents: Optional[int] = None

    def remaining(self, now: Optional[datetime] = None) -> timedelta:
        if self.billing_cycle_end is None:
            return timedelta(0)
        if now is None:
            now = datetime.now(timezone.utc)
        if now.tzinfo is None:
            now = now.replace(tzinfo=timezone.utc)
        end = self.billing_cycle_end
        if end.tzinfo is None:
            end = end.replace(tzinfo=timezone.utc)
        delta = end - now
        return delta if delta.total_seconds() > 0 else timedelta(0)
