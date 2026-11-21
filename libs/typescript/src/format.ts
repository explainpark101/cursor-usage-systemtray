export function formatUsd(cents: number): string {
  const neg = cents < 0;
  const abs = Math.abs(cents);
  const s = `$${Math.floor(abs / 100)}.${String(abs % 100).padStart(2, "0")}`;
  return neg ? `-${s}` : s;
}

export function formatUsdOptional(cents: number | null | undefined): string {
  if (cents == null) return "—";
  return formatUsd(cents);
}

/** Format duration ms as `Dd Hh Mm Ss`. */
export function formatRemaining(ms: number): string {
  const totalSec = Math.max(0, Math.floor(ms / 1000));
  const days = Math.floor(totalSec / (60 * 60 * 24));
  const hours = Math.floor((totalSec % (60 * 60 * 24)) / (60 * 60));
  const mins = Math.floor((totalSec % (60 * 60)) / 60);
  const secs = totalSec % 60;
  return `${days}d ${hours}h ${mins}m ${secs}s`;
}

export function formatLocalTime(d: Date | null | undefined): string {
  if (!d) return "—";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
