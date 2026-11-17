import {
  ChangeDetectionStrategy,
  Component,
  DOCUMENT,
  InjectionToken,
  OnInit,
  computed,
  inject,
  signal,
} from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { InitiatePaymentResponse } from '../payments.models';
import { PaymentsApiError, PaymentsApiService } from '../payments-api.service';
import { MoneyPipe } from '../shared/money.pipe';
import { PaymentsShellComponent } from '../shared/payments-shell/payments-shell.component';

export const LAST_PAYMENT_ATTEMPT_STORAGE_KEY = 'cb_last_payment_attempt_id';

export const FORM_SUBMITTER = new InjectionToken<(form: HTMLFormElement) => void>(
  'FORM_SUBMITTER',
  {
    providedIn: 'root',
    factory: () => (form: HTMLFormElement) => form.submit(),
  },
);

@Component({
  selector: 'app-payment-redirect',
  standalone: true,
  imports: [RouterLink, MoneyPipe, PaymentsShellComponent],
  templateUrl: './payment-redirect.component.html',
  styleUrl: './payment-redirect.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PaymentRedirectComponent implements OnInit {
  private readonly route = inject(ActivatedRoute);
  private readonly paymentsApi = inject(PaymentsApiService);
  private readonly doc = inject(DOCUMENT);
  private readonly submitForm = inject(FORM_SUBMITTER);

  readonly orderId = signal<string>('');
  readonly loading = signal<boolean>(true);
  readonly errorStatus = signal<number | null>(null);
  readonly errorMessage = signal<string | null>(null);
  readonly paymentSession = signal<InitiatePaymentResponse | null>(null);

  readonly displayAmountCents = computed<number>(() => {
    const session = this.paymentSession();
    const rawAmount = session?.checkout?.fields?.['amount'] ?? '';
    return parseDecimalStringToCents(rawAmount);
  });

  readonly displayCurrency = computed<string>(() => {
    const session = this.paymentSession();
    return session?.checkout?.fields?.['currency'] || 'USD';
  });

  ngOnInit(): void {
    const paramOrderId = (this.route.snapshot.paramMap.get('orderId') ?? '').trim();
    this.orderId.set(paramOrderId);
    if (!paramOrderId) {
      this.loading.set(false);
      this.errorStatus.set(404);
      this.errorMessage.set("We couldn't find that order");
      return;
    }
    this.loadCheckoutSession();
  }

  loadCheckoutSession(): void {
    const currentOrderId = this.orderId();
    if (!currentOrderId) {
      return;
    }

    this.loading.set(true);
    this.errorStatus.set(null);
    this.errorMessage.set(null);

    this.paymentsApi.initiatePayment(currentOrderId).subscribe({
      next: (res) => {
        this.paymentSession.set(res);
        this.persistAttemptId(res.attempt_id);
        this.loading.set(false);
      },
      error: (err: unknown) => {
        this.loading.set(false);
        const status = err instanceof PaymentsApiError ? err.status : 0;
        this.errorStatus.set(status);
        this.errorMessage.set(this.mapErrorStatusToMessage(status));
      },
    });
  }

  continueToPayHere(): void {
    const session = this.paymentSession();
    if (!session || !session.checkout) {
      return;
    }

    const form = this.buildHiddenCheckoutForm(session);
    this.doc.body.appendChild(form);
    this.submitForm(form);
  }

  buildHiddenCheckoutForm(session: InitiatePaymentResponse): HTMLFormElement {
    const form = this.doc.createElement('form');
    form.method = (session.checkout.method || 'POST').toUpperCase();
    form.action = session.checkout.checkout_url;
    form.style.display = 'none';

    const fields = session.checkout.fields ?? {};
    for (const [key, value] of Object.entries(fields)) {
      const input = this.doc.createElement('input');
      input.type = 'hidden';
      input.name = key;
      input.value = String(value ?? '');
      form.appendChild(input);
    }

    return form;
  }

  private persistAttemptId(attemptId: string): void {
    if (!attemptId) {
      return;
    }
    try {
      sessionStorage.setItem(LAST_PAYMENT_ATTEMPT_STORAGE_KEY, attemptId);
    } catch {
      // Ignore storage quota or privacy-mode exceptions.
    }
  }

  private mapErrorStatusToMessage(status: number): string {
    switch (status) {
      case 404:
        return "We couldn't find that order";
      case 409:
        return 'This order is no longer awaiting payment';
      case 503:
        return 'Payments are temporarily unavailable';
      default:
        return 'We could not prepare your payment session right now. Please try again.';
    }
  }
}

function parseDecimalStringToCents(raw: string): number {
  const trimmed = (raw || '').trim();
  if (!trimmed) {
    return 0;
  }
  const parts = trimmed.split('.');
  const whole = parseInt(parts[0] || '0', 10);
  if (!Number.isFinite(whole) || whole < 0) {
    return 0;
  }
  const fracRaw = (parts[1] || '').padEnd(2, '0').slice(0, 2);
  const frac = parseInt(fracRaw || '0', 10);
  return whole * 100 + (Number.isFinite(frac) && frac >= 0 ? frac : 0);
}
