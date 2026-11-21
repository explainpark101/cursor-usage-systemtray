use std::time::{Duration, SystemTime, UNIX_EPOCH};

#[derive(Debug, Clone, Default)]
pub struct PlanUsage {
    pub total_percent_used: f64,
    pub auto_percent_used: f64,
    pub api_percent_used: f64,
}

#[derive(Debug, Clone, Default)]
pub struct Snapshot {
    pub plan_usage: PlanUsage,
    pub membership_type: String,
    pub plan_name: String,
    pub plan_price: String,
    pub display_message: String,
    /// Unix epoch milliseconds (UTC).
    pub billing_cycle_start_ms: Option<i64>,
    /// Unix epoch milliseconds (UTC) — subscription / included-usage renewal.
    pub billing_cycle_end_ms: Option<i64>,
    pub total_spend_cents: i64,
    pub included_spend_cents: i64,
    pub bonus_spend_cents: i64,
    pub plan_limit_cents: i64,
    pub included_enabled: bool,
    pub included_used_cents: i64,
    pub included_limit_cents: i64,
    pub included_remaining_cents: i64,
    pub included_bonus_cents: i64,
    pub included_breakdown_total: i64,
    pub on_demand_enabled: bool,
    pub on_demand_used_cents: i64,
    pub on_demand_limit_cents: Option<i64>,
    pub on_demand_remaining_cents: Option<i64>,
}

impl Snapshot {
    pub fn remaining_ms(&self) -> i64 {
        let end = match self.billing_cycle_end_ms {
            Some(v) => v,
            None => return 0,
        };
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|d| d.as_millis() as i64)
            .unwrap_or(0);
        (end - now).max(0)
    }

    pub fn remaining(&self) -> Duration {
        Duration::from_millis(self.remaining_ms() as u64)
    }
}
