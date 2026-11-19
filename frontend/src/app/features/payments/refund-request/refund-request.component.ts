import { DatePipe } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  OnInit,
  computed,
  inject,
  signal,
} from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { forkJoin } from 'rxjs';
import {
  BuyerPayment,
  BuyerRefund,
  RefundReason,
  RefundStatus,
  RefundTimelineStep,
  RefundTimelineStepKey,
} from '../payments.models';
import { PaymentsApiError, PaymentsApiService } from '../payments-api.service';
import { AccountShellComponent } from '../shared/account-shell/account-shell.component';
import { MoneyPipe, formatMoneyCents } from '../shared/money.pipe';

// TODO: Server is authoritative for REFUND_WINDOW_DAYS; this constant provides client-side eligibility guidance.
export const REFUND_WINDOW_DAYS_DEFAULT = 30;

export interface RefundReasonOption {
  value: RefundReason;
  label: string;
}

export interface TimelineDisplayRow {
  key: RefundTimelineStepKey;
  title: string;
  description: string;
  at?: string;
  done: boolean;
}

/**
 * Parses a positive decimal amount string with up to 2 fractional digits into integer cents
 * without floating-point division or multiplication. Returns null if invalid.
 */
export function parseRefundDecimalToCents(raw: string | null | undefined): number | null {
  const trimmed = (raw ?? '').trim();
  if (!trimmed || !/^\d+(\.\d{1,2})?$/.test(trimmed)) {
    return null;
  }
  const parts = trimmed.split('.');
  const whole = parseInt(parts[0], 10);
  if (!Number.isFinite(whole) || whole < 0) {
    return null;
  }
  const fracStr = (parts[1] ?? '').padEnd(2, '0');
  const frac = fracStr ? parseInt(fracStr, 10) : 0;
  if (!Number.isFinite(frac) || frac < 0 || frac > 99) {
    return null;
  }
  return whole * 100 + frac;
}

export function formatCentsToPlainInput(cents: number): string {
  const safe = Math.max(0, Math.trunc(cents));
  const whole = Math.trunc(safe / 100);
  const frac = String(safe % 100).padStart(2, '0');
  return `${whole}.${frac}`;
}

@Component({
  selector: 'app-refund-request',
  standalone: true,
  imports: [DatePipe, AccountShellComponent, MoneyPipe],
  templateUrl: './refund-request.component.html',
  styleUrl: './refund-request.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class RefundRequestComponent implements OnInit {
  private readonly paymentsApi = inject(PaymentsApiService);
  private readonly route = inject(ActivatedRoute);

  readonly reasonOptions: readonly RefundReasonOption[] = [
    { value: 'ARRIVED_DAMAGED', label: 'Arrived damaged' },
    { value: 'NOT_AS_DESCRIBED', label: 'Not as described' },
    { value: 'WRONG_ITEM', label: 'Wrong item received' },
    { value: 'LATE_DELIVERY', label: 'Late delivery' },
    { value: 'CHANGED_MIND', label: 'Changed my mind' },
    { value: 'OTHER', label: 'Other' },
  ];

  readonly loading = signal<boolean>(true);
  readonly loadError = signal<string | null>(null);

  // TODO(member-04): Replace paid-payments order source with Member 04's buyer orders endpoint once merged.
  readonly paidOrders = signal<BuyerPayment[]>([]);
  readonly refunds = signal<BuyerRefund[]>([]);
  readonly selectedRefund = signal<BuyerRefund | null>(null);

  // Form state signals
  readonly selectedOrderId = signal<string>('');
  readonly reason = signal<RefundReason | ''>('ARRIVED_DAMAGED');
  readonly details = signal<string>('');
  readonly amountInput = signal<string>('');
  readonly formTouched = signal<boolean>(false);

  // Submission & idempotency state
  readonly currentIdempotencyKey = signal<string>(generateUuid());
  readonly submitting = signal<boolean>(false);
  readonly submitError = signal<string | null>(null);
  readonly submitSuccessMessage = signal<string | null>(null);
  readonly replayMessage = signal<string | null>(null);

  readonly selectedPaidOrder = computed<BuyerPayment | null>(() => {
    const id = this.selectedOrderId();
    return this.paidOrders().find((o) => o.order_id === id) ?? null;
  });

  readonly selectedOrderCurrency = computed<string>(() => {
    return this.selectedPaidOrder()?.currency || 'USD';
  });

  readonly isWithinRefundWindow = computed<boolean>(() => {
    const order = this.selectedPaidOrder();
    if (!order) {
      return true;
    }
    const refIso = order.completed_at || order.created_at;
    const paidMs = Date.parse(refIso);
    if (!Number.isFinite(paidMs)) {
      return true;
    }
    const windowMs = REFUND_WINDOW_DAYS_DEFAULT * 24 * 60 * 60 * 1000;
    return Date.now() - paidMs <= windowMs;
  });

  readonly maxRefundableCents = computed<number>(() => {
    const order = this.selectedPaidOrder();
    if (!order) {
      return 0;
    }
    const usedCents = this.refunds()
      .filter((r) => r.order_id === order.order_id && r.status !== 'REJECTED')
      .reduce((acc, r) => acc + (r.amount_cents || 0), 0);
    return Math.max(0, order.amount_cents - usedCents);
  });

  readonly detailsLength = computed<number>(() => this.details().trim().length);

  readonly detailsValidationError = computed<string | null>(() => {
    const len = this.detailsLength();
    if (len === 0) {
      return 'Please describe what happened with your order (10–500 characters).';
    }
    if (len < 10) {
      return 'Details must be at least 10 characters.';
    }
    if (len > 500) {
      return 'Details must not exceed 500 characters.';
    }
    return null;
  });

  readonly parsedAmountCents = computed<number | null>(() => {
    return parseRefundDecimalToCents(this.amountInput());
  });

  readonly amountValidationError = computed<string | null>(() => {
    const cents = this.parsedAmountCents();
    if (cents === null) {
      return 'Enter a valid amount with up to 2 decimal places (e.g. 20.00).';
    }
    if (cents <= 0) {
      return 'Refund amount must be greater than $0.00.';
    }
    const max = this.maxRefundableCents();
    if (cents > max) {
      return `Amount cannot exceed the refundable balance of ${formatMoneyCents(max, this.selectedOrderCurrency())}.`;
    }
    return null;
  });

  readonly reasonValidationError = computed<string | null>(() => {
    return this.reason() ? null : 'Please select a reason for your refund.';
  });

  readonly activeTimelineRows = computed<TimelineDisplayRow[]>(() => {
    const ref = this.selectedRefund();
    const apiSteps = ref?.timeline ?? [];
    const byKey = new Map<RefundTimelineStepKey, RefundTimelineStep>();
    for (const s of apiSteps) {
      byKey.set(s.step, s);
    }

    const status = ref?.status ?? 'PENDING';
    const approvedDone =
      byKey.get('approved')?.done ??
      (status === 'APPROVED' || status === 'PROCESSED' || status === 'COMPLETED');
    const issuedDone = byKey.get('refund_issued')?.done ?? status === 'COMPLETED';

    return [
      {
        key: 'submitted',
        title: 'Request submitted',
        description: 'Your refund request has been received.',
        at: byKey.get('submitted')?.at ?? ref?.requested_at,
        done: byKey.get('submitted')?.done ?? true,
      },
      {
        key: 'under_review',
        title: 'Under review',
        description: "We're reviewing your request and will get back to you soon.",
        at: byKey.get('under_review')?.at ?? ref?.decided_at ?? ref?.requested_at,
        done: byKey.get('under_review')?.done ?? true,
      },
      {
        key: 'approved',
        title: 'Refund approved',
        description: "We'll let you know once your refund is approved.",
        at: byKey.get('approved')?.at ?? (approvedDone ? ref?.decided_at : undefined),
        done: approvedDone,
      },
      {
        key: 'refund_issued',
        title: 'Refund issued',
        description: "You'll be notified once your refund has been processed.",
        at: byKey.get('refund_issued')?.at ?? (issuedDone ? ref?.completed_at : undefined),
        done: issuedDone,
      },
    ];
  });

  ngOnInit(): void {
    this.loadInitialData();
  }

  loadInitialData(): void {
    this.loading.set(true);
    this.loadError.set(null);

    const preselectedOrderParam = (
      this.route.snapshot?.queryParamMap?.get('order') ?? ''
    ).trim();

    forkJoin({
      payments: this.paymentsApi.listPayments(1, 100, 'SUCCESS'),
      refunds: this.paymentsApi.listRefunds(1, 100),
    }).subscribe({
      next: ({ payments, refunds }) => {
        const uniqueOrders = deduplicatePaidOrders(payments.data ?? []);
        this.paidOrders.set(uniqueOrders);

        const refundList = refunds.data ?? [];
        this.refunds.set(refundList);
        if (refundList.length > 0 && !this.selectedRefund()) {
          this.selectedRefund.set(refundList[0]);
        }

        if (
          preselectedOrderParam &&
          uniqueOrders.some((o) => o.order_id === preselectedOrderParam)
        ) {
          this.selectOrder(preselectedOrderParam);
        } else if (uniqueOrders.length > 0) {
          this.selectOrder(uniqueOrders[0].order_id);
        }

        this.loading.set(false);
      },
      error: (err: unknown) => {
        this.loading.set(false);
        const msg =
          err instanceof PaymentsApiError
            ? err.message
            : 'We could not load your returns and refunds right now.';
        this.loadError.set(msg || 'We could not load your returns and refunds right now.');
      },
    });
  }

  selectOrder(orderId: string): void {
    if (this.selectedOrderId() !== orderId) {
      this.selectedOrderId.set(orderId);
      this.regenerateIdempotencyKey();
      const maxCents = this.maxRefundableCents();
      this.amountInput.set(maxCents > 0 ? formatCentsToPlainInput(maxCents) : '');
    }
  }

  onReasonChange(value: string): void {
    this.reason.set(value as RefundReason);
    this.regenerateIdempotencyKey();
  }

  onDetailsInput(value: string): void {
    this.details.set(value);
  }

  onAmountInput(value: string): void {
    this.amountInput.set(value);
    this.regenerateIdempotencyKey();
  }

  selectExistingRefund(ref: BuyerRefund): void {
    this.selectedRefund.set(ref);
  }

  submitRefundRequest(): void {
    this.formTouched.set(true);
    this.submitError.set(null);
    this.submitSuccessMessage.set(null);
    this.replayMessage.set(null);

    const orderId = this.selectedOrderId();
    const selectedReason = this.reason();
    const detailsErr = this.detailsValidationError();
    const amountErr = this.amountValidationError();
    const reasonErr = this.reasonValidationError();
    const cents = this.parsedAmountCents();

    if (!orderId || !selectedReason || detailsErr || amountErr || reasonErr || cents === null) {
      return;
    }

    this.submitting.set(true);
    const formattedAmount = formatCentsToPlainInput(cents);

    this.paymentsApi
      .createRefundRequest(
        orderId,
        {
          reason: selectedReason,
          details: this.details().trim(),
          amount: formattedAmount,
        },
        this.currentIdempotencyKey(),
      )
      .subscribe({
        next: ({ refund, replayed }) => {
          this.submitting.set(false);
          this.selectedRefund.set(refund);

          const existingIdx = this.refunds().findIndex((r) => r.id === refund.id);
          if (existingIdx >= 0) {
            const copy = [...this.refunds()];
            copy[existingIdx] = refund;
            this.refunds.set(copy);
          } else {
            this.refunds.set([refund, ...this.refunds()]);
          }

          if (replayed) {
            this.replayMessage.set('You already submitted this request');
          } else {
            this.submitSuccessMessage.set('Your refund request has been submitted.');
            this.details.set('');
            this.formTouched.set(false);
            this.regenerateIdempotencyKey();
            const remaining = this.maxRefundableCents();
            this.amountInput.set(remaining > 0 ? formatCentsToPlainInput(remaining) : '');
          }
        },
        error: (err: unknown) => {
          this.submitting.set(false);
          // Notice: currentIdempotencyKey is intentionally preserved so retrying reuses the exact same key.
          const status = err instanceof PaymentsApiError ? err.status : 0;
          const backendMsg = err instanceof PaymentsApiError ? err.message : '';
          this.submitError.set(this.mapSubmitError(status, backendMsg));
        },
      });
  }

  statusBannerTitle(status: RefundStatus): string {
    switch (status) {
      case 'PENDING':
        return 'Under review';
      case 'APPROVED':
        return 'Approved';
      case 'PROCESSED':
        return 'Refund processing';
      case 'COMPLETED':
        return 'Refund issued';
      case 'REJECTED':
        return 'Request declined';
    }
  }

  statusBannerSubtitle(status: RefundStatus): string {
    switch (status) {
      case 'PENDING':
        return 'We received your request and are currently reviewing the details.';
      case 'APPROVED':
        return 'Your refund was approved and is being processed.';
      case 'PROCESSED':
        return 'Your refund is on its way.';
      case 'COMPLETED':
        return 'Your refund has been processed.';
      case 'REJECTED':
        return 'We were unable to approve this request.';
    }
  }

  statusBannerClasses(status: RefundStatus): string {
    switch (status) {
      case 'COMPLETED':
        return 'bg-emerald-50 border-emerald-200 text-emerald-950';
      case 'APPROVED':
      case 'PROCESSED':
        return 'bg-amber-50/90 border-amber-200 text-amber-950';
      case 'REJECTED':
        return 'bg-red-50 border-red-200 text-red-950';
      default:
        return 'bg-terracotta-100/70 border-terracotta/25 text-ink';
    }
  }

  formatReasonLabel(reason: string): string {
    const found = this.reasonOptions.find((r) => r.value === reason);
    return found ? found.label : reason;
  }

  private regenerateIdempotencyKey(): void {
    this.currentIdempotencyKey.set(generateUuid());
  }

  private mapSubmitError(status: number, backendMessage: string): string {
    const lower = (backendMessage || '').toLowerCase();
    if (status === 409) {
      if (lower.includes('window')) {
        return 'The 30-day refund window for this order has expired.';
      }
      return 'This order does not have a verified payment eligible for a refund.';
    }
    if (status === 422) {
      return 'The requested refund amount exceeds the remaining refundable balance for this order.';
    }
    if (status === 404) {
      return "We couldn't find that order.";
    }
    if (status === 403) {
      return 'You do not have permission to request a refund for this order.';
    }
    return backendMessage || 'We could not submit your refund request right now. Please try again.';
  }
}

function deduplicatePaidOrders(payments: BuyerPayment[]): BuyerPayment[] {
  const byOrder = new Map<string, BuyerPayment>();
  for (const p of payments) {
    if (p.status !== 'SUCCESS') {
      continue;
    }
    if (!byOrder.has(p.order_id)) {
      byOrder.set(p.order_id, p);
    }
  }
  return Array.from(byOrder.values());
}

function generateUuid(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return 'idem-' + Math.random().toString(36).slice(2, 11) + '-' + Date.now().toString(36);
}
