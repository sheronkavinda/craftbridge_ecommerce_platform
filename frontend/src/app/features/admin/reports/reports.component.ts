import { CommonModule } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  OnInit,
  computed,
  inject,
  signal,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import {
  buildCsvContent,
  downloadCsvFile,
  formatCsvRangeSuffix,
} from '../../../shared/csv.util';
import {
  CurrencyLkrPipe,
  formatCurrencyLkr,
} from '../../../shared/pipes/currency-lkr.pipe';
import {
  DailyRevenuePoint,
  RevenueSummary,
  SellerSalesReport,
} from '../admin.models';
import { AdminService } from '../admin.service';

export type DateRangePreset = 'LAST_7_DAYS' | 'LAST_30_DAYS' | 'THIS_MONTH' | 'ALL_TIME' | 'CUSTOM';

export type SellerSortColumn =
  | 'store_name'
  | 'order_count'
  | 'verified_payments'
  | 'refunds'
  | 'net_revenue';

export interface ChartBarGeometry {
  point: DailyRevenuePoint;
  index: number;
  x: number;
  width: number;
  grossY: number;
  grossHeight: number;
  netY: number;
  netHeight: number;
  isNegativeNet: boolean;
  showXLabel: boolean;
  shortDateLabel: string;
}

export interface ChartYTick {
  value: number;
  y: number;
  label: string;
}

@Component({
  selector: 'app-admin-reports',
  standalone: true,
  imports: [CommonModule, FormsModule, CurrencyLkrPipe],
  templateUrl: './reports.component.html',
  styleUrl: './reports.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ReportsComponent implements OnInit {
  private readonly adminService = inject(AdminService);
  private readonly destroyRef = inject(DestroyRef);

  // SVG coordinate system constants
  readonly svgWidth = 760;
  readonly svgHeight = 260;
  readonly padLeft = 72;
  readonly padRight = 20;
  readonly padTop = 20;
  readonly padBottom = 38;

  // Date range inputs & active state
  readonly fromDateInput = signal<string>('');
  readonly toDateInput = signal<string>('');
  readonly appliedFromDate = signal<string>('');
  readonly appliedToDate = signal<string>('');
  readonly activePreset = signal<DateRangePreset>('ALL_TIME');
  readonly dateValidationError = signal<string>('');

  // Section 1: Revenue Summary
  readonly revenueSummary = signal<RevenueSummary | null>(null);
  readonly isSummaryLoading = signal<boolean>(false);
  readonly summaryError = signal<string>('');

  // Section 2: Daily Revenue Breakdown
  readonly dailyRevenue = signal<DailyRevenuePoint[]>([]);
  readonly isDailyLoading = signal<boolean>(false);
  readonly dailyError = signal<string>('');
  readonly activeChartPoint = signal<DailyRevenuePoint | null>(null);

  // Section 3: Seller Sales Table
  readonly sellerReports = signal<SellerSalesReport[]>([]);
  readonly isSellersLoading = signal<boolean>(false);
  readonly sellersError = signal<string>('');
  readonly sortColumn = signal<SellerSortColumn>('net_revenue');
  readonly sortDirection = signal<'asc' | 'desc'>('desc');

  // Screen-reader polite announcement
  readonly liveAnnouncement = signal<string>('');

  readonly isAnyLoading = computed<boolean>(() => {
    return this.isSummaryLoading() || this.isDailyLoading() || this.isSellersLoading();
  });

  readonly chronologicalDailyPoints = computed<DailyRevenuePoint[]>(() => {
    const items = [...this.dailyRevenue()];
    return items.sort((a, b) => a.date.localeCompare(b.date));
  });

  readonly hasDailyRevenueData = computed<boolean>(() => {
    const pts = this.chronologicalDailyPoints();
    if (pts.length === 0) {
      return false;
    }
    return pts.some(
      (p) =>
        (Number(p.verified_payments) || 0) !== 0 ||
        (Number(p.refunds) || 0) !== 0 ||
        (Number(p.net_revenue) || 0) !== 0,
    );
  });

  readonly chartLayout = computed<{
    bars: ChartBarGeometry[];
    yTicks: ChartYTick[];
    zeroY: number;
  }>(() => {
    const pts = this.chronologicalDailyPoints();
    const plotWidth = this.svgWidth - this.padLeft - this.padRight;
    const plotHeight = this.svgHeight - this.padTop - this.padBottom;

    if (pts.length === 0) {
      return {
        bars: [],
        yTicks: [],
        zeroY: this.padTop + plotHeight,
      };
    }

    let rawMin = 0;
    let rawMax = 0;
    for (const pt of pts) {
      const gross = Number(pt.verified_payments) || 0;
      const net = Number(pt.net_revenue) || 0;
      rawMax = Math.max(rawMax, gross, net);
      rawMin = Math.min(rawMin, gross, net, 0);
    }

    if (rawMax === 0 && rawMin === 0) {
      rawMax = 1000;
    }

    const niceMax = this.ceilToNiceBound(rawMax);
    const niceMin = rawMin < 0 ? -this.ceilToNiceBound(Math.abs(rawMin)) : 0;
    const span = Math.max(1, niceMax - niceMin);

    const valueToY = (val: number): number => {
      const ratio = (niceMax - val) / span;
      return this.padTop + ratio * plotHeight;
    };

    const zeroY = valueToY(0);

    const tickCount = 5;
    const yTicks: ChartYTick[] = [];
    for (let i = 0; i < tickCount; i++) {
      const ratio = i / (tickCount - 1);
      const val = niceMax - ratio * span;
      yTicks.push({
        value: val,
        y: this.padTop + ratio * plotHeight,
        label: this.formatAbbreviatedLkr(val),
      });
    }

    const count = pts.length;
    const slotWidth = plotWidth / count;
    const barWidth = Math.max(8, Math.min(42, slotWidth * 0.62));
    const labelStride = count <= 7 ? 1 : Math.ceil(count / 6);

    const bars: ChartBarGeometry[] = pts.map((pt, idx) => {
      const centerX = this.padLeft + slotWidth * idx + slotWidth / 2;
      const x = centerX - barWidth / 2;

      const grossVal = Math.max(0, Number(pt.verified_payments) || 0);
      const grossTop = valueToY(grossVal);
      const grossHeight = Math.max(2, Math.abs(zeroY - grossTop));
      const grossY = Math.min(zeroY, grossTop);

      const netVal = Number(pt.net_revenue) || 0;
      const netTopOrBottom = valueToY(netVal);
      const netHeight = Math.max(2, Math.abs(zeroY - netTopOrBottom));
      const netY = netVal >= 0 ? netTopOrBottom : zeroY;

      const showXLabel = idx === 0 || idx === count - 1 || idx % labelStride === 0;

      return {
        point: pt,
        index: idx,
        x,
        width: barWidth,
        grossY,
        grossHeight,
        netY,
        netHeight,
        isNegativeNet: netVal < 0,
        showXLabel,
        shortDateLabel: this.formatShortDate(pt.date),
      };
    });

    return { bars, yTicks, zeroY };
  });

  readonly chartAriaSummary = computed<string>(() => {
    const pts = this.chronologicalDailyPoints();
    if (pts.length === 0) {
      return 'Daily revenue chart: no revenue data in the selected period.';
    }
    const first = pts[0];
    const last = pts[pts.length - 1];
    const totalNet = pts.reduce((acc, p) => acc + (Number(p.net_revenue) || 0), 0);
    return `Daily revenue bar chart from ${first.date} to ${last.date} (${pts.length} days). Total net revenue across plotted days is ${formatCurrencyLkr(totalNet)}.`;
  });

  readonly sortedSellers = computed<SellerSalesReport[]>(() => {
    const list = [...this.sellerReports()];
    const col = this.sortColumn();
    const dir = this.sortDirection() === 'asc' ? 1 : -1;

    return list.sort((a, b) => {
      if (col === 'store_name') {
        return a.store_name.localeCompare(b.store_name) * dir;
      }
      const valA = Number(a[col]) || 0;
      const valB = Number(b[col]) || 0;
      if (valA === valB) {
        return a.store_name.localeCompare(b.store_name);
      }
      return (valA - valB) * dir;
    });
  });

  readonly sellerTotals = computed<{
    orderCount: number;
    verifiedPayments: number;
    refunds: number;
    netRevenue: number;
  }>(() => {
    let orderCount = 0;
    let verifiedPayments = 0;
    let refunds = 0;
    let netRevenue = 0;

    for (const s of this.sellerReports()) {
      orderCount += Number(s.order_count) || 0;
      verifiedPayments += Number(s.verified_payments) || 0;
      refunds += Number(s.refunds) || 0;
      netRevenue += Number(s.net_revenue) || 0;
    }

    return { orderCount, verifiedPayments, refunds, netRevenue };
  });

  ngOnInit(): void {
    this.loadAllReports();
  }

  onFromDateChange(value: string): void {
    this.fromDateInput.set(value);
    this.activePreset.set('CUSTOM');
    this.validateDateInputs(value, this.toDateInput());
  }

  onToDateChange(value: string): void {
    this.toDateInput.set(value);
    this.activePreset.set('CUSTOM');
    this.validateDateInputs(this.fromDateInput(), value);
  }

  applyDateRange(): void {
    const from = this.fromDateInput().trim();
    const to = this.toDateInput().trim();
    if (!this.validateDateInputs(from, to)) {
      return;
    }
    this.appliedFromDate.set(from);
    this.appliedToDate.set(to);
    if (!from && !to) {
      this.activePreset.set('ALL_TIME');
    }
    this.loadAllReports();
  }

  selectPreset(preset: DateRangePreset): void {
    const today = new Date();
    let from = '';
    let to = '';

    if (preset === 'LAST_7_DAYS') {
      const start = new Date(today);
      start.setDate(today.getDate() - 6);
      from = this.toIsoDateString(start);
      to = this.toIsoDateString(today);
    } else if (preset === 'LAST_30_DAYS') {
      const start = new Date(today);
      start.setDate(today.getDate() - 29);
      from = this.toIsoDateString(start);
      to = this.toIsoDateString(today);
    } else if (preset === 'THIS_MONTH') {
      const start = new Date(today.getFullYear(), today.getMonth(), 1);
      from = this.toIsoDateString(start);
      to = this.toIsoDateString(today);
    } else if (preset === 'ALL_TIME') {
      from = '';
      to = '';
    }

    this.activePreset.set(preset);
    this.fromDateInput.set(from);
    this.toDateInput.set(to);
    this.dateValidationError.set('');
    this.appliedFromDate.set(from);
    this.appliedToDate.set(to);
    this.loadAllReports();
  }

  refreshReports(): void {
    this.loadAllReports();
  }

  loadAllReports(): void {
    this.liveAnnouncement.set('');
    this.loadSummary();
    this.loadDailyRevenue();
    this.loadSellerRevenue();
  }

  loadSummary(): void {
    this.isSummaryLoading.set(true);
    this.summaryError.set('');

    this.adminService
      .getRevenueSummary(this.appliedFromDate(), this.appliedToDate())
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (summary) => {
          this.revenueSummary.set(summary ?? null);
          this.isSummaryLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
        error: (err: unknown) => {
          this.summaryError.set(
            this.extractErrorMessage(err, 'Failed to load revenue summary.'),
          );
          this.isSummaryLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
      });
  }

  loadDailyRevenue(): void {
    this.isDailyLoading.set(true);
    this.dailyError.set('');
    this.activeChartPoint.set(null);

    this.adminService
      .getDailyRevenue(this.appliedFromDate(), this.appliedToDate(), 90)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (points) => {
          const list = points ?? [];
          this.dailyRevenue.set(list);
          if (list.length > 0) {
            this.activeChartPoint.set(list[0]);
          }
          this.isDailyLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
        error: (err: unknown) => {
          this.dailyError.set(
            this.extractErrorMessage(err, 'Failed to load daily revenue breakdown.'),
          );
          this.isDailyLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
      });
  }

  loadSellerRevenue(): void {
    this.isSellersLoading.set(true);
    this.sellersError.set('');

    this.adminService
      .getSellerRevenue(this.appliedFromDate(), this.appliedToDate(), 100)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (sellers) => {
          this.sellerReports.set(sellers ?? []);
          this.isSellersLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
        error: (err: unknown) => {
          this.sellersError.set(
            this.extractErrorMessage(err, 'Failed to load seller sales report.'),
          );
          this.isSellersLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
      });
  }

  selectChartPoint(point: DailyRevenuePoint): void {
    this.activeChartPoint.set(point);
  }

  toggleSort(column: SellerSortColumn): void {
    if (this.sortColumn() === column) {
      this.sortDirection.update((dir) => (dir === 'asc' ? 'desc' : 'asc'));
    } else {
      this.sortColumn.set(column);
      this.sortDirection.set(column === 'store_name' ? 'asc' : 'desc');
    }
  }

  getAriaSort(column: SellerSortColumn): 'ascending' | 'descending' | 'none' {
    if (this.sortColumn() !== column) {
      return 'none';
    }
    return this.sortDirection() === 'asc' ? 'ascending' : 'descending';
  }

  exportDailyRevenueCsv(): void {
    const points = this.chronologicalDailyPoints();
    if (points.length === 0) {
      return;
    }

    const headers = ['date', 'verified_payments', 'refunds', 'net_revenue'];
    const rows = points.map((pt) => [
      pt.date,
      Number(pt.verified_payments) || 0,
      Number(pt.refunds) || 0,
      Number(pt.net_revenue) || 0,
    ]);

    const csv = buildCsvContent(headers, rows, true);
    const suffix = formatCsvRangeSuffix(this.appliedFromDate(), this.appliedToDate());
    downloadCsvFile(`craftbridge-daily-revenue_${suffix}.csv`, csv);
  }

  exportSellerRevenueCsv(): void {
    const sellers = this.sortedSellers();
    if (sellers.length === 0) {
      return;
    }

    const headers = [
      'seller_id',
      'store_name',
      'order_count',
      'verified_payments',
      'refunds',
      'net_revenue',
    ];
    const rows = sellers.map((s) => [
      s.seller_id,
      s.store_name,
      Number(s.order_count) || 0,
      Number(s.verified_payments) || 0,
      Number(s.refunds) || 0,
      Number(s.net_revenue) || 0,
    ]);

    const csv = buildCsvContent(headers, rows, true);
    const suffix = formatCsvRangeSuffix(this.appliedFromDate(), this.appliedToDate());
    downloadCsvFile(`craftbridge-seller-revenue_${suffix}.csv`, csv);
  }

  formatAbbreviatedLkr(value: number): string {
    const num = Number(value) || 0;
    const sign = num < 0 ? '-' : '';
    const abs = Math.abs(num);

    if (abs >= 1_000_000) {
      return `${sign}LKR ${(abs / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`;
    }
    if (abs >= 1_000) {
      return `${sign}LKR ${(abs / 1_000).toFixed(1).replace(/\.0$/, '')}K`;
    }
    return `${sign}LKR ${abs.toFixed(0)}`;
  }

  private validateDateInputs(from: string, to: string): boolean {
    const f = from.trim();
    const t = to.trim();
    if (f && t && f > t) {
      this.dateValidationError.set('From date must be on or before To date.');
      return false;
    }
    this.dateValidationError.set('');
    return true;
  }

  private toIsoDateString(date: Date): string {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
  }

  private formatShortDate(isoDate: string): string {
    if (!isoDate || isoDate.length < 10) {
      return isoDate;
    }
    return isoDate.slice(5); // MM-DD
  }

  private ceilToNiceBound(val: number): number {
    if (val <= 0) {
      return 1000;
    }
    const exponent = Math.floor(Math.log10(val));
    const magnitude = Math.pow(10, exponent);
    const residual = val / magnitude;
    let niceResidual = 10;
    if (residual <= 1.2) {
      niceResidual = 1.2;
    } else if (residual <= 2) {
      niceResidual = 2;
    } else if (residual <= 2.5) {
      niceResidual = 2.5;
    } else if (residual <= 5) {
      niceResidual = 5;
    }
    return niceResidual * magnitude;
  }

  private checkAndAnnounceCompletion(): void {
    if (!this.isAnyLoading()) {
      this.liveAnnouncement.set('Reports updated');
    }
  }

  private extractErrorMessage(err: unknown, fallback: string): string {
    if (err instanceof HttpErrorResponse) {
      const serverError = err.error?.error ?? err.error?.message;
      if (typeof serverError === 'string' && serverError.trim().length > 0) {
        return serverError.trim();
      }
    }
    if (err && typeof err === 'object' && 'error' in err) {
      const nested = (err as { error?: { error?: string; message?: string } }).error;
      if (typeof nested?.error === 'string' && nested.error.trim().length > 0) {
        return nested.error.trim();
      }
      if (typeof nested?.message === 'string' && nested.message.trim().length > 0) {
        return nested.message.trim();
      }
    }
    return fallback;
  }
}
