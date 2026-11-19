import { DatePipe } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  OnInit,
  computed,
  inject,
  signal,
} from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import {
  SellerEarningsSummary,
  SellerPayout,
  SellerTransaction,
} from '../payments.models';
import { PaymentsApiError, PaymentsApiService } from '../payments-api.service';
import { MoneyPipe } from '../shared/money.pipe';
import { SellerShellComponent } from '../shared/seller-shell/seller-shell.component';

export type SellerEarningsTab = 'earnings' | 'payouts';
export type SellerRangeDays = 7 | 30 | 90;

const DEFAULT_PAGE_SIZE = 20;

/**
 * Derives the platform commission percentage for a transaction.
 * TODO: Expose platform_commission_percent directly from the GET /api/v1/seller/earnings response.
 */
export function deriveCommissionPercent(tx: SellerTransaction | null | undefined): number {
  if (!tx || tx.gross_cents <= 0) {
    return 10;
  }
  return Math.round((tx.commission_cents * 100) / tx.gross_cents);
}

@Component({
  selector: 'app-seller-earnings',
  standalone: true,
  imports: [DatePipe, MoneyPipe, SellerShellComponent],
  templateUrl: './seller-earnings.component.html',
  styleUrl: './seller-earnings.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SellerEarningsComponent implements OnInit {
  private readonly paymentsApi = inject(PaymentsApiService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);

  readonly pageSize = DEFAULT_PAGE_SIZE;
  readonly rangeOptions: ReadonlyArray<{ value: SellerRangeDays; label: string }> = [
    { value: 7, label: 'Last 7 days' },
    { value: 30, label: 'Last 30 days' },
    { value: 90, label: 'Last 90 days' },
  ];

  readonly activeTab = signal<SellerEarningsTab>('earnings');
  readonly rangeDays = signal<SellerRangeDays>(30);
  readonly page = signal<number>(1);
  readonly total = signal<number>(0);

  readonly loading = signal<boolean>(true);
  readonly errorStatus = signal<number | null>(null);
  readonly errorMessage = signal<string | null>(null);

  readonly summary = signal<SellerEarningsSummary>({
    pending_cents: 0,
    available_cents: 0,
    paid_cents: 0,
    currency: 'USD',
  });
  readonly transactions = signal<SellerTransaction[]>([]);
  readonly payouts = signal<SellerPayout[]>([]);
  readonly selectedTransaction = signal<SellerTransaction | null>(null);

  readonly currency = computed<string>(() => this.summary().currency || 'USD');

  readonly selectedCommissionPct = computed<number>(() =>
    deriveCommissionPercent(this.selectedTransaction()),
  );

  readonly tableCommissionPct = computed<number>(() => {
    const first = this.transactions()[0];
    return deriveCommissionPercent(first);
  });

  readonly totalPages = computed<number>(() => {
    const t = this.total();
    return t > 0 ? Math.ceil(t / this.pageSize) : 1;
  });

  readonly hasPrevious = computed<boolean>(() => this.page() > 1);
  readonly hasNext = computed<boolean>(() => this.page() < this.totalPages());

  ngOnInit(): void {
    const qp = this.route.snapshot?.queryParamMap;
    const rawTab = (qp?.get('tab') ?? '').trim().toLowerCase();
    if (rawTab === 'payouts' || rawTab === 'earnings') {
      this.activeTab.set(rawTab);
    }

    const rawRange = parseInt(qp?.get('range_days') ?? '30', 10);
    if (rawRange === 7 || rawRange === 30 || rawRange === 90) {
      this.rangeDays.set(rawRange);
    }

    const rawPage = parseInt(qp?.get('page') ?? '1', 10);
    if (Number.isFinite(rawPage) && rawPage >= 1) {
      this.page.set(rawPage);
    }

    this.loadEarnings();
  }

  selectTab(tab: SellerEarningsTab): void {
    if (this.activeTab() === tab) {
      return;
    }
    this.activeTab.set(tab);
    this.syncQueryParams();
  }

  onRangeChange(rawVal: string | number): void {
    const parsed = Number(rawVal);
    if (parsed !== 7 && parsed !== 30 && parsed !== 90) {
      return;
    }
    if (this.rangeDays() === parsed && this.page() === 1) {
      return;
    }
    this.rangeDays.set(parsed);
    this.page.set(1);
    this.syncQueryParams();
    this.loadEarnings();
  }

  goToPage(targetPage: number): void {
    if (targetPage < 1 || targetPage > this.totalPages() || targetPage === this.page()) {
      return;
    }
    this.page.set(targetPage);
    this.syncQueryParams();
    this.loadEarnings();
  }

  selectTransaction(tx: SellerTransaction): void {
    this.selectedTransaction.set(tx);
  }

  loadEarnings(): void {
    this.loading.set(true);
    this.errorStatus.set(null);
    this.errorMessage.set(null);

    this.paymentsApi
      .getSellerEarnings(this.rangeDays(), this.page(), this.pageSize)
      .subscribe({
        next: (res) => {
          this.summary.set(
            res.data?.summary ?? {
              pending_cents: 0,
              available_cents: 0,
              paid_cents: 0,
              currency: 'USD',
            },
          );
          const txList = res.data?.transactions ?? [];
          this.transactions.set(txList);
          this.payouts.set(res.data?.payouts ?? []);
          this.total.set(res.meta?.total ?? txList.length);
          this.selectedTransaction.set(txList[0] ?? null);
          this.loading.set(false);
        },
        error: (err: unknown) => {
          this.loading.set(false);
          const status = err instanceof PaymentsApiError ? err.status : 0;
          this.errorStatus.set(status);
          if (status === 403) {
            this.errorMessage.set('Seller access required');
          } else {
            const msg =
              err instanceof PaymentsApiError
                ? err.message
                : 'We could not load your seller earnings right now.';
            this.errorMessage.set(
              msg || 'We could not load your seller earnings right now.',
            );
          }
        },
      });
  }

  commissionPctFor(tx: SellerTransaction): number {
    return deriveCommissionPercent(tx);
  }

  negativeIfPositive(cents: number): number {
    return cents > 0 ? -cents : 0;
  }

  private syncQueryParams(): void {
    const tab = this.activeTab();
    const range = this.rangeDays();
    const currentPage = this.page();

    void this.router.navigate([], {
      relativeTo: this.route,
      queryParams: {
        tab: tab === 'payouts' ? 'payouts' : null,
        range_days: range !== 30 ? range : null,
        page: currentPage > 1 ? currentPage : null,
      },
      queryParamsHandling: 'merge',
    });
  }
}
