import { DatePipe } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  OnInit,
  computed,
  inject,
  signal,
} from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { BuyerPayment, PaymentStatus } from '../payments.models';
import { PaymentsApiError, PaymentsApiService } from '../payments-api.service';
import { AccountShellComponent } from '../shared/account-shell/account-shell.component';
import { MoneyPipe } from '../shared/money.pipe';
import { formatProviderLabel } from '../shared/payment-labels';
import { PaymentStatusChipComponent } from '../shared/payment-status-chip/payment-status-chip.component';

const DEFAULT_PAGE_SIZE = 10;

export interface StatusFilterTab {
  label: string;
  value: PaymentStatus | '';
}

const VALID_STATUSES: ReadonlySet<string> = new Set([
  'PENDING',
  'SUCCESS',
  'FAILED',
  'CANCELLED',
  'EXPIRED',
]);

@Component({
  selector: 'app-payment-history',
  standalone: true,
  imports: [
    DatePipe,
    RouterLink,
    AccountShellComponent,
    MoneyPipe,
    PaymentStatusChipComponent,
  ],
  templateUrl: './payment-history.component.html',
  styleUrl: './payment-history.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PaymentHistoryComponent implements OnInit {
  private readonly paymentsApi = inject(PaymentsApiService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);

  readonly pageSize = DEFAULT_PAGE_SIZE;

  readonly filterTabs: readonly StatusFilterTab[] = [
    { label: 'All', value: '' },
    { label: 'Paid', value: 'SUCCESS' },
    { label: 'Pending', value: 'PENDING' },
    { label: 'Failed', value: 'FAILED' },
    { label: 'Cancelled', value: 'CANCELLED' },
    { label: 'Expired', value: 'EXPIRED' },
  ];

  readonly skeletonRows = [1, 2, 3, 4];

  readonly selectedStatus = signal<PaymentStatus | ''>('');
  readonly page = signal<number>(1);
  readonly total = signal<number>(0);
  readonly payments = signal<BuyerPayment[]>([]);
  readonly loading = signal<boolean>(true);
  readonly errorMessage = signal<string | null>(null);

  readonly totalPages = computed<number>(() => {
    const t = this.total();
    return t > 0 ? Math.ceil(t / this.pageSize) : 1;
  });

  readonly rangeStart = computed<number>(() => {
    const t = this.total();
    if (t === 0 || this.payments().length === 0) {
      return 0;
    }
    return (this.page() - 1) * this.pageSize + 1;
  });

  readonly rangeEnd = computed<number>(() => {
    const t = this.total();
    if (t === 0 || this.payments().length === 0) {
      return 0;
    }
    return Math.min(t, this.rangeStart() + this.payments().length - 1);
  });

  readonly hasPrevious = computed<boolean>(() => this.page() > 1);
  readonly hasNext = computed<boolean>(() => this.page() < this.totalPages());

  ngOnInit(): void {
    const qp = this.route.snapshot?.queryParamMap;
    const rawStatus = (qp?.get('status') ?? '').trim().toUpperCase();
    if (VALID_STATUSES.has(rawStatus)) {
      this.selectedStatus.set(rawStatus as PaymentStatus);
    }

    const rawPage = parseInt(qp?.get('page') ?? '1', 10);
    if (Number.isFinite(rawPage) && rawPage >= 1) {
      this.page.set(rawPage);
    }

    this.loadPayments();
  }

  selectStatusFilter(status: PaymentStatus | ''): void {
    if (this.selectedStatus() === status && this.page() === 1) {
      return;
    }
    this.selectedStatus.set(status);
    this.page.set(1);
    this.syncQueryParams();
    this.loadPayments();
  }

  goToPage(targetPage: number): void {
    if (targetPage < 1 || targetPage > this.totalPages() || targetPage === this.page()) {
      return;
    }
    this.page.set(targetPage);
    this.syncQueryParams();
    this.loadPayments();
  }

  loadPayments(): void {
    this.loading.set(true);
    this.errorMessage.set(null);

    const statusFilter = this.selectedStatus() || undefined;
    this.paymentsApi.listPayments(this.page(), this.pageSize, statusFilter).subscribe({
      next: (res) => {
        this.payments.set(res.data ?? []);
        this.total.set(res.meta?.total ?? 0);
        this.loading.set(false);
      },
      error: (err: unknown) => {
        this.loading.set(false);
        const msg =
          err instanceof PaymentsApiError
            ? err.message
            : 'We could not load your payment history right now.';
        this.errorMessage.set(msg || 'We could not load your payment history right now.');
      },
    });
  }

  formatMethod(provider: string | null | undefined): string {
    return formatProviderLabel(provider);
  }

  private syncQueryParams(): void {
    const status = this.selectedStatus();
    const currentPage = this.page();
    void this.router.navigate([], {
      relativeTo: this.route,
      queryParams: {
        status: status || null,
        page: currentPage > 1 ? currentPage : null,
      },
      queryParamsHandling: 'merge',
    });
  }
}
