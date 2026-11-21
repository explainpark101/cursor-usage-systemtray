# @cursor-tray/cursor-api (TypeScript)

Unofficial TypeScript/Node client for Cursor dashboard usage / billing data.
Uses native `fetch` (Node 18+). No runtime dependencies.

## Install

```bash
npm install ./libs/typescript
# after build:
cd libs/typescript && bunx tsc -p tsconfig.json
```

## Usage

```ts
import { parseCookieInput, fetchSnapshot, formatUsd, formatRemaining } from "@cursor-tray/cursor-api";

const cookie = parseCookieInput(process.env.CURSOR_COOKIE!);
const snap = await fetchSnapshot(cookie);
console.log(snap.planName, formatRemaining(snap.remainingMs()));
console.log(formatUsd(snap.totalSpendCents));
```
