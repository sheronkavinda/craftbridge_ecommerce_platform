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
import { RouterLink } from '@angular/router';
import { CurrencyLkrPipe } from '../../../shared/pipes/currency-lkr.pipe';
import {
  AuditLog,
  RefundRecord,
  RevenueSummary,
  SellerApplication,
  UserSummary,
} from '../admin.models';
import { AdminService } from '../admin.service';

@Component({
  selector: 'app-admin-dashboard',
  standalone: true,
  imports: [CommonModule, RouterLink, CurrencyLkrPipe],
  templateUrl: './admin-dashboard.component.html',
  styleUrl: './admin-dashboard.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class AdminDashboardComponent implements OnInit {
  private readonly adminService = inject(AdminService);
  private readonly destroyRef = inject(DestroyRef);

  // Section 1: Revenue Summary
  readonly revenueSummary = signal<RevenueSummary | null>(null);
  readonly isRevenueLoading = signal<boolean>(false);
  readonly revenueError = signal<string>('');

  // Section 2a: Seller Applications (Action Needed)
  readonly sellerApplications = signal<SellerApplication[]>([]);
  readonly isSellersLoading = signal<boolean>(false);
  readonly sellersError = signal<string>('');

  // Section 2b: Refunds (Action Needed)
  readonly refunds = signal<RefundRecord[]>([]);
  readonly isRefundsLoading = signal<boolean>(false);
  readonly refundsError = signal<string>('');

  // Section 2c: Users (Action Needed)
  readonly users = signal<UserSummary[]>([]);
  readonly isUsersLoading = signal<boolean>(false);
  readonly usersError = signal<string>('');

  // Section 3: Recent Activity (Audit Logs)
  readonly recentAuditLogs = signal<AuditLog[]>([]);
  readonly isAuditLoading = signal<boolean>(false);
  readonly auditError = signal<string>('');

  // Screen-reader live announcement
  readonly liveAnnouncement = signal<string>('');

  readonly isAnyLoading = computed<boolean>(() => {
    return (
      this.isRevenueLoading() ||
      this.isSellersLoading() ||
      this.isRefundsLoading() ||
      this.isUsersLoading() ||
      this.isAuditLoading()
    );
  });

  readonly pendingSellerCount = computed<number>(() => {
    let count = 0;
    for (const app of this.sellerApplications()) {
      if ((app.status ?? '').trim().toUpperCase() === 'PENDING') {
        count++;
      }
    }
    return count;
  });

  readonly pendingRefundStats = computed<{ count: number; totalAmount: number }>(() => {
    let count = 0;
    let totalAmount = 0;
    for (const r of this.refunds()) {
      if ((r.status ?? '').trim().toUpperCase() === 'PENDING') {
        count++;
        totalAmount += Number(r.amount) || 0;
      }
    }
    return { count, totalAmount };
  });

  readonly suspendedUserCount = computed<number>(() => {
    let count = 0;
    for (const u of this.users()) {
      const st = (u.status ?? '').trim().toUpperCase();
      if (st === 'SUSPENDED' || st === 'BANNED') {
        count++;
      }
    }
    return count;
  });

  ngOnInit(): void {
    this.refreshAll();
  }

  refreshAll(): void {
    this.liveAnnouncement.set('');
    this.loadRevenue();
    this.loadSellers();
    this.loadRefunds();
    this.loadUsers();
    this.loadRecentAuditLogs();
  }

  loadRevenue(): void {
    this.isRevenueLoading.set(true);
    this.revenueError.set('');

    this.adminService
      .getRevenueSummary()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (summary) => {
          this.revenueSummary.set(summary ?? null);
          this.isRevenueLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
        error: (err: unknown) => {
          this.revenueError.set(
            this.extractErrorMessage(err, 'Failed to load revenue summary.'),
          );
          this.isRevenueLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
      });
  }

  loadSellers(): void {
    this.isSellersLoading.set(true);
    this.sellersError.set('');

    this.adminService
      .listSellerApplications('', 200, 0)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (items) => {
          this.sellerApplications.set(items ?? []);
          this.isSellersLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
        error: (err: unknown) => {
          this.sellersError.set(
            this.extractErrorMessage(err, 'Failed to load seller applications.'),
          );
          this.isSellersLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
      });
  }

  loadRefunds(): void {
    this.isRefundsLoading.set(true);
    this.refundsError.set('');

    this.adminService
      .listRefunds('', 200, 0)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (items) => {
          this.refunds.set(items ?? []);
          this.isRefundsLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
        error: (err: unknown) => {
          this.refundsError.set(this.extractErrorMessage(err, 'Failed to load refunds.'));
          this.isRefundsLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
      });
  }

  loadUsers(): void {
    this.isUsersLoading.set(true);
    this.usersError.set('');

    this.adminService
      .listUsers('', '', 200, 0)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (items) => {
          this.users.set(items ?? []);
          this.isUsersLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
        error: (err: unknown) => {
          this.usersError.set(this.extractErrorMessage(err, 'Failed to load users.'));
          this.isUsersLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
      });
  }

  loadRecentAuditLogs(): void {
    this.isAuditLoading.set(true);
    this.auditError.set('');

    this.adminService
      .listAuditLogs('', '', '', 5, 0)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (items) => {
          this.recentAuditLogs.set((items ?? []).slice(0, 5));
          this.isAuditLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
        error: (err: unknown) => {
          this.auditError.set(
            this.extractErrorMessage(err, 'Failed to load recent activity.'),
          );
          this.isAuditLoading.set(false);
          this.checkAndAnnounceCompletion();
        },
      });
  }

  formatActionLabel(action: string): string {
    const raw = (action ?? '').trim();
    if (!raw) {
      return 'Admin action';
    }
    const normalized = raw.replace(/[_-]+/g, ' ').toLowerCase();
    return normalized.charAt(0).toUpperCase() + normalized.slice(1);
  }

  formatRelativeTime(isoDate: string): string {
    if (!isoDate) {
      return '';
    }
    const timestamp = Date.parse(isoDate);
    if (Number.isNaN(timestamp)) {
      return '';
    }
    const diffMs = Date.now() - timestamp;
    const diffSec = Math.floor(diffMs / 1000);

    if (diffSec < 45) {
      return 'Just now';
    }
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) {
      return `${diffMin}m ago`;
    }
    const diffHours = Math.floor(diffMin / 60);
    if (diffHours < 24) {
      return `${diffHours}h ago`;
    }
    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 30) {
      return `${diffDays}d ago`;
    }
    return '';
  }

  private checkAndAnnounceCompletion(): void {
    if (!this.isAnyLoading()) {
      this.liveAnnouncement.set('Dashboard updated');
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

