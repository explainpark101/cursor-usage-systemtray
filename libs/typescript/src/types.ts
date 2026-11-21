export interface PlanUsage {
  totalPercentUsed: number;
  autoPercentUsed: number;
  apiPercentUsed: number;
}

export interface Snapshot {
  planUsage: PlanUsage;
  membershipType: string;
  planName: string;
  planPrice: string;
  displayMessage: string;
  billingCycleStart: Date | null;
  billingCycleEnd: Date | null;
  totalSpendCents: number;
  includedSpendCents: number;
  bonusSpendCents: number;
  planLimitCents: number;
  includedEnabled: boolean;
  includedUsedCents: number;
  includedLimitCents: number;
  includedRemainingCents: number;
  includedBonusCents: number;
  includedBreakdownTotal: number;
  onDemandEnabled: boolean;
  onDemandUsedCents: number;
  onDemandLimitCents: number | null;
  onDemandRemainingCents: number | null;
  /** Milliseconds until billingCycleEnd (clamped at 0). */
  remainingMs(now?: Date): number;
}

export function emptySnapshot(): Snapshot {
  const snap: Snapshot = {
    planUsage: { totalPercentUsed: 0, autoPercentUsed: 0, apiPercentUsed: 0 },
    membershipType: "",
    planName: "",
    planPrice: "",
    displayMessage: "",
    billingCycleStart: null,
    billingCycleEnd: null,
    totalSpendCents: 0,
    includedSpendCents: 0,
    bonusSpendCents: 0,
    planLimitCents: 0,
    includedEnabled: false,
    includedUsedCents: 0,
    includedLimitCents: 0,
    includedRemainingCents: 0,
    includedBonusCents: 0,
    includedBreakdownTotal: 0,
    onDemandEnabled: false,
    onDemandUsedCents: 0,
    onDemandLimitCents: null,
    onDemandRemainingCents: null,
    remainingMs(now = new Date()) {
      if (!this.billingCycleEnd) return 0;
      return Math.max(0, this.billingCycleEnd.getTime() - now.getTime());
    },
  };
  return snap;
}
