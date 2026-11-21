/** Parse Cookie header or Netscape cookies.txt into a Cookie header string. */
export function parseCookieInput(raw: string): string {
  raw = raw.trim();
  if (!raw) throw new Error("cookie input is empty");
  if (looksLikeNetscape(raw)) {
    const header = parseNetscape(raw);
    if (!header) throw new Error("no cursor.com cookies found in cookie file");
    return header;
  }
  const header = trimHeader(raw);
  if (!header) throw new Error("cookie value is empty");
  return normalizeHeader(header);
}

function normalizeHeader(header: string): string {
  if (header.toLowerCase().startsWith("cookie:")) {
    header = header.slice("cookie:".length).trim();
  }
  if (!header.includes("=") && !header.includes(";") && header) {
    return `WorkosCursorSessionToken=${header}`;
  }
  return header;
}

function looksLikeNetscape(raw: string): boolean {
  const lower = raw.toLowerCase();
  if (
    lower.includes("netscape http cookie file") ||
    lower.includes("# httponly_") ||
    raw.includes("#HttpOnly_")
  ) {
    return true;
  }
  for (const line of raw.split("\n")) {
    const t = line.trim();
    if (!t || t.startsWith("#")) continue;
    if ((line.match(/\t/g) ?? []).length >= 6) return true;
    const fields = t.split(/\s+/);
    if (fields.length >= 7 && (fields[1] === "TRUE" || fields[1] === "FALSE")) return true;
  }
  return false;
}

function isCursorDomain(domain: string): boolean {
  const d = domain.replace(/^\./, "").toLowerCase();
  return d === "cursor.com" || d.endsWith(".cursor.com");
}

function parseNetscape(raw: string): string {
  const now = Math.floor(Date.now() / 1000);
  const parts: string[] = [];
  const seen = new Set<string>();
  let parsed = 0;

  for (let line of raw.split("\n")) {
    line = line.replace(/\r$/, "");
    if (!line) continue;
    if (line.startsWith("#HttpOnly_")) line = line.slice("#HttpOnly_".length);
    else if (line.trimStart().startsWith("#")) continue;

    let fields = line.split("\t");
    if (fields.length < 7) {
      fields = line.trim().split(/\s+/);
      if (fields.length < 7) continue;
      if (fields.length > 7) fields = [...fields.slice(0, 6), fields.slice(6).join(" ")];
    } else if (fields.length > 7) {
      fields = [...fields.slice(0, 6), fields.slice(6).join("\t")];
    }

    const domain = fields[0]!.trim();
    const name = fields[5]!.trim();
    const value = fields[6]!;
    if (!domain || !name || !isCursorDomain(domain)) continue;
    const expStr = fields[4]!.trim();
    if (expStr && expStr !== "0") {
      const exp = Number(expStr);
      if (Number.isFinite(exp) && exp > 0 && exp < now) continue;
    }
    if (seen.has(name)) {
      const idx = parts.findIndex((p) => p.startsWith(name + "="));
      if (idx >= 0) parts.splice(idx, 1);
    }
    seen.add(name);
    parts.push(`${name}=${value}`);
    parsed++;
  }

  if (parsed === 0 && parts.length === 0) {
    if (hasAnyCookieLine(raw)) return "";
    throw new Error("no cookie lines found (expected Netscape cookies.txt)");
  }
  return parts.join("; ");
}

function hasAnyCookieLine(raw: string): boolean {
  for (let line of raw.split("\n")) {
    line = line.replace(/\r$/, "");
    if (line.startsWith("#HttpOnly_")) return true;
    if (!line.trim() || line.trimStart().startsWith("#")) continue;
    if ((line.match(/\t/g) ?? []).length >= 6 || line.trim().split(/\s+/).length >= 7) return true;
  }
  return false;
}

function trimHeader(s: string): string {
  return s.replace(/[\n\r\t]/g, " ").replace(/ {2,}/g, " ").trim();
}
