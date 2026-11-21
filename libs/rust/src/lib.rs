//! Unofficial client for cursor.com dashboard usage / billing APIs.

mod client;
mod cookies;
mod format;
mod types;

pub use client::{fetch_snapshot, Error, Unauthorized};
pub use cookies::parse_cookie_input;
pub use format::{format_local_time, format_remaining, format_remaining_duration, format_usd};
pub use types::{PlanUsage, Snapshot};
