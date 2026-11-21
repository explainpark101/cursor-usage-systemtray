import { test } from "node:test";
import assert from "node:assert/strict";
import { formatRemaining, formatUsd } from "./format.ts";
import { buildSnapshot } from "./client.ts";

test("formatUsd", () => {
  assert.equal(formatUsd(7000), "$70.00");
  assert.equal(formatUsd(75693), "$756.93");
});

test("formatRemaining", () => {
  assert.equal(formatRemaining(((26 * 60 + 5) * 60 + 7) * 1000), "1d 2h 5m 7s");
});

test("buildSnapshot", () => {
  const s = buildSnapshot(
    {
      membershipType: "pro_plus",
      individualUsage: {
        plan: {
          enabled: true,
          used: 7000,
          limit: 7000,
          remaining: 0,
          breakdown: { included: 7000, bonus: 75693, total: 82693 },
          totalPercentUsed: 67.3,
          autoPercentUsed: 68.9,
          apiPercentUsed: 0,
        },
        onDemand: { enabled: false, used: 0, limit: null, remaining: null },
      },
    },
    {
      planUsage: {
        totalSpend: 82693,
        includedSpend: 7000,
        bonusSpend: 75693,
        limit: 7000,
      },
      displayMessage: "hit",
    },
    { planInfo: { planName: "Pro+", price: "$60/mo", renewsAt: "1795277729000" } },
  );
  assert.equal(s.planName, "Pro+");
  assert.equal(s.totalSpendCents, 82693);
  assert.equal(s.onDemandEnabled, false);
  assert.equal(s.billingCycleEnd?.getTime(), 1795277729000);
});
