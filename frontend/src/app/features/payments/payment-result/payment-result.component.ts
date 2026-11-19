import { DatePipe } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  OnInit,
  computed,
  inject,
  signal,
} from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { Subscription, interval, switchMap } from 'rxjs';
import { LAST_PAYMENT_ATTEMPT_STORAGE_KEY } from '../payment-redirect/payment-redirect.component';
import { BuyerPayment } from '../payments.models';
import { PaymentsApiService } from '../payments-api.service';
import { MoneyPipe } from '../shared/money.pipe';
import { PaymentsShellComponent } from '../shared/payments-shell/payments-shell.component';

const POLL_INTERVAL_MS = 3000;
const MAX_POLL_DURATION_MS = 120_000;
const MAX_POLL_TICKS = Math.floor(MAX_POLL_DURATION_MS / POLL_INTERVAL_MS);

@Component({
  selector: 'app-payment-result',
  standalone: true,
  imports: [DatePipe, RouterLink, MoneyPipe, PaymentsShellComponent],
  templateUrl: './payment-result.component.html',
  styleUrl: './payment-result.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PaymentResultComponent implements OnInit {
  private readonly route = inject(ActivatedRoute);
  private readonly paymentsApi = inject(PaymentsApiService);
  private readonly destroyRef = inject(DestroyRef);

  private pollSub: Subscription | null = null;
  private pollTicks = 0;

  readonly loading = signal<boolean>(true);
  readonly payment = signal<BuyerPayment | null>(null);
  readonly notFoundError = signal<string | null>(null);
  readonly timedOut = signal<boolean>(false);

  readonly providerLabel = computed<string>(() => {
    const p = this.payment();
    const raw = (p?.provider ?? '').toLowerCase();
    if (!raw || raw === 'payhere_sandbox' || raw === 'payhere') {
      return 'PayHere Sandbox';
    }
    return p?.provider ?? 'PayHere Sandbox';
  });

  readonly failureHeading = computed<string>(() => {
    const st = this.payment()?.status;
    switch (st) {
      case 'CANCELLED':
        return 'Payment was cancelled';
      case 'EXPIRED':
        return 'This payment session expired';
      default:
        return "Your payment didn't go through";
    }
  });

  constructor() {
    this.destroyRef.onDestroy(() => {
      this.stopPolling();
    });
  }

  ngOnInit(): void {
    this.resolveAndLoadPayment();
  }

  checkAgain(): void {
    const current = this.payment();
    this.timedOut.set(false);
    if (current?.id) {
      this.fetchPaymentById(current.id, true);
    } else {
      this.resolveAndLoadPayment();
    }
  }

  private resolveAndLoadPayment(): void {
    this.stopPolling();
    this.loading.set(true);
    this.notFoundError.set(null);
    this.timedOut.set(false);

    // Note: URL query params NEVER decide payment outcome; only getPayment() status does.
    const qp = this.route.snapshot.queryParamMap;
    const paramAttempt = (qp.get('attempt') ?? qp.get('order_id') ?? '').trim();

    if (paramAttempt) {
      this.fetchPaymentById(paramAttempt, true);
      return;
    }

    const storedAttempt = this.readSessionAttemptId();
    if (storedAttempt) {
      this.fetchPaymentById(storedAttempt, true);
      return;
    }

    this.loadLatestFromHistory();
  }

  private fetchPaymentById(attemptId: string, startPollIfPending: boolean): void {
    this.paymentsApi.getPayment(attemptId).subscribe({
      next: (item) => {
        this.applyPaymentState(item, startPollIfPending);
      },
      error: () => {
        this.loading.set(false);
        this.notFoundError.set("We couldn't find that payment record.");
      },
    });
  }

  private loadLatestFromHistory(): void {
    this.paymentsApi.listPayments(1, 1).subscribe({
      next: (res) => {
        const newest = res.data?.[0];
        if (!newest) {
          this.loading.set(false);
          this.notFoundError.set('No recent payment attempts found.');
          return;
        }
        this.fetchPaymentById(newest.id, true);
      },
      error: () => {
        this.loading.set(false);
        this.notFoundError.set("We couldn't load your payment status right now.");
      },
    });
  }

  private applyPaymentState(item: BuyerPayment, startPollIfPending: boolean): void {
    this.payment.set(item);
    this.loading.set(false);

    if (item.status === 'PENDING') {
      if (startPollIfPending) {
        this.startPolling(item.id);
      }
    } else {
      this.stopPolling();
    }
  }

  private startPolling(attemptId: string): void {
    this.stopPolling();
    this.pollTicks = 0;

    this.pollSub = interval(POLL_INTERVAL_MS)
      .pipe(
        switchMap(() => {
          this.pollTicks++;
          return this.paymentsApi.getPayment(attemptId);
        }),
      )
      .subscribe({
        next: (updated) => {
          this.payment.set(updated);
          if (updated.status !== 'PENDING') {
            this.stopPolling();
            return;
          }
          if (this.pollTicks >= MAX_POLL_TICKS) {
            this.timedOut.set(true);
            this.stopPolling();
          }
        },
        error: () => {
          if (this.pollTicks >= MAX_POLL_TICKS) {
            this.timedOut.set(true);
            this.stopPolling();
          }
        },
      });
  }

  private stopPolling(): void {
    if (this.pollSub) {
      this.pollSub.unsubscribe();
      this.pollSub = null;
    }
  }

  private readSessionAttemptId(): string {
    try {
      return (sessionStorage.getItem(LAST_PAYMENT_ATTEMPT_STORAGE_KEY) ?? '').trim();
    } catch {
      return '';
    }
  }
}
