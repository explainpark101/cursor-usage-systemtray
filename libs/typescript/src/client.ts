import { emptySnapshot, type Snapshot, type PlanUsage } from "./types.js";

export const USAGE_SUMMARY_URL = "https://cursor.com/api/usage-summary";
export const PERIOD_USAGE_URL = "https://cursor.com/api/dashboard/get-current-period-usage";
export const PLAN_INFO_URL = "https://cursor.com/api/dashboard/get-plan-info";

const UA =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36";

export class UnauthorizedError extends Error {
  constructor(message = "unauthorized (HTTP 401/403)") {
    super(message);
    this.name = "UnauthorizedError";
  }
}

export async function fetchSnapshot(
  cookie: string,
  opts: { timeoutMs?: number; fetchImpl?: typeof fetch } = {},
): Promise<Snapshot> {
  if (!cookie) throw new Error("no cookie configured");
  const timeoutMs = opts.timeoutMs ?? 8000;
  const f = opts.fetchImpl ?? fetch;

  const [summaryRes, periodRes, planRes] = await Promise.allSettled([
    request(f, "GET", USAGE_SUMMARY_URL, cookie, undefined, timeoutMs),
    request(f, "POST", PERIOD_USAGE_URL, cookie, "{}", timeoutMs),
    request(f, "POST", PLAN_INFO_URL, cookie, "{}", timeoutMs),
  ]);

  for (const r of [summaryRes, periodRes, planRes]) {
    if (r.status === "rejected" && r.reason instanceof UnauthorizedError) throw r.reason;
  }
  if (summaryRes.status !== "fulfilled") {
    throw summaryRes.reason instanceof Error
      ? summaryRes.reason
      : new Error("empty usage-summary");
  }
  const period = periodRes.status === "fulfilled" ? periodRes.value : null;
  const plan = planRes.status === "fulfilled" ? planRes.value : null;
  return buildSnapshot(summaryRes.value, period, plan);
}

async function request(
  f: typeof fetch,
  method: string,
  url: string,
  cookie: string,
  body: string | undefined,
  timeoutMs: number,
): Promise<unknown> {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const headers: Record<string, string> = {
      Accept: "application/json, text/plain, */*",
      Origin: "https://cursor.com",
      Referer: "https://cursor.com/dashboard",
      "User-Agent": UA,
      Cookie: cookie,
    };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    const res = await f(url, { method, headers, body, signal: ctrl.signal });
    const text = await res.text();
    if (res.status === 401 || res.status === 403) {
      if (text.includes("Invalid origin")) {
        throw new Error(`rejected by CSRF origin check: ${text.slice(0, 120)}`);
      }
      throw new UnauthorizedError(`HTTP ${res.status}`);
    }
    if (!res.ok) throw new Error(`HTTP ${res.status}: ${text.slice(0, 120)}`);
    return JSON.parse(text) as unknown;
  } finally {
    clearTimeout(t);
  }
}

function parseTime(value: unknown): Date | null {
  if (value == null) return null;
  if (typeof value === "number") return new Date(value);
  if (typeof value === "string") {
    const s = value.trim();
    if (!s) return null;
    const asNum = Number(s);
    if (Number.isFinite(asNum) && /^\d+(\.\d+)?$/.test(s)) return new Date(asNum);
    const d = new Date(s);
    return Number.isNaN(d.getTime()) ? null : d;
  }
  return null;
}

function firstNonEmpty(...vals: Array<string | undefined | null>): string {
  for (const v of vals) {
    if (v && v.trim()) return v;
  }
  return "";
}

/** @internal exported for tests */
export function buildSnapshot(
  summary: any,
  period: any | null,
  plan: any | null,
): Snapshot {
  const s = emptySnapshot();
  s.membershipType = summary.membershipType ?? "";
  s.displayMessage = firstNonEmpty(
    period?.displayMessage,
    period?.autoModelSelectedDisplayMessage,
    summary.autoModelSelectedDisplayMessage,
  );
  s.billingCycleStart = parseTime(summary.billingCycleStart);
  s.billingCycleEnd = parseTime(summary.billingCycleEnd);

  const planInfo = plan?.planInfo;
  if (planInfo) {
    s.planName = planInfo.planName ?? "";
    s.planPrice = planInfo.price ?? "";
    const renew = parseTime(planInfo.renewsAt) ?? parseTime(planInfo.billingCycleEnd);
    if (renew) s.billingCycleEnd = renew;
  }

  if (period) {
    if (!s.billingCycleStart) s.billingCycleStart = parseTime(period.billingCycleStart);
    if (!s.billingCycleEnd) s.billingCycleEnd = parseTime(period.billingCycleEnd);
    const pu = period.planUsage;
    if (pu) {
      s.totalSpendCents = Number(pu.totalSpend ?? 0);
      s.includedSpendCents = Number(pu.includedSpend ?? 0);
      s.bonusSpendCents = Number(pu.bonusSpend ?? 0);
      s.planLimitCents = Number(pu.limit ?? 0);
      s.planUsage = {
        totalPercentUsed: Number(pu.totalPercentUsed ?? 0),
        autoPercentUsed: Number(pu.autoPercentUsed ?? 0),
        apiPercentUsed: Number(pu.apiPercentUsed ?? 0),
      } satisfies PlanUsage;
    }
    const slu = period.spendLimitUsage;
    if (slu) {
      if (slu.individualUsed != null) s.onDemandUsedCents = Number(slu.individualUsed);
      if (slu.individualLimit != null) s.onDemandLimitCents = Number(slu.individualLimit);
      if (slu.individualRemaining != null) {
        s.onDemandRemainingCents = Number(slu.individualRemaining);
      }
    }
  }

  const iu = summary.individualUsage ?? {};
  const p = iu.plan;
  if (p) {
    s.includedEnabled = !!p.enabled;
    s.includedUsedCents = Number(p.used ?? 0);
    s.includedLimitCents = Number(p.limit ?? 0);
    s.includedRemainingCents = Number(p.remaining ?? 0);
    const bd = p.breakdown;
    if (bd) {
      s.includedBonusCents = Number(bd.bonus ?? 0);
      s.includedBreakdownTotal = Number(bd.total ?? 0);
      if (!s.includedSpendCents) s.includedSpendCents = Number(bd.included ?? 0);
      if (!s.bonusSpendCents) s.bonusSpendCents = Number(bd.bonus ?? 0);
      if (!s.totalSpendCents) s.totalSpendCents = Number(bd.total ?? 0);
    }
    if (!s.planLimitCents) s.planLimitCents = Number(p.limit ?? 0);
    s.planUsage = {
      totalPercentUsed: Number(p.totalPercentUsed ?? 0),
      autoPercentUsed: Number(p.autoPercentUsed ?? 0),
      apiPercentUsed: Number(p.apiPercentUsed ?? 0),
    };
  }
  const od = iu.onDemand;
  if (od) {
    s.onDemandEnabled = !!od.enabled;
    s.onDemandUsedCents = Number(od.used ?? 0);
    s.onDemandLimitCents = od.limit == null ? null : Number(od.limit);
    s.onDemandRemainingCents = od.remaining == null ? null : Number(od.remaining);
  }
  return s;
}
