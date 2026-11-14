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
  DashboardCategoryStats,
  DashboardSummary,
  DashboardUserStats,
  RevenueSummary,
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

  readonly dashboard = signal<DashboardSummary | null>(null);
  readonly isLoading = signal<boolean>(false);
  readonly loadError = signal<string>('');
  readonly liveAnnouncement = signal<string>('');

  readonly isAnyLoading = computed<boolean>(() => this.isLoading());

  readonly revenueSummary = computed<RevenueSummary | null>(() => {
    return this.dashboard()?.revenue ?? null;
  });

  readonly pendingSellerCount = computed<number>(() => {
    return this.dashboard()?.sellers?.pending ?? 0;
  });

  readonly pendingRefundStats = computed<{ count: number; totalAmount: number }>(() => {
    const refunds = this.dashboard()?.refunds;
    return {
      count: refunds?.pending_count ?? 0,
      totalAmount: Number(refunds?.pending_amount) || 0,
    };
  });

  readonly suspendedUserCount = computed<number>(() => {
    return this.dashboard()?.users?.suspended ?? 0;
  });

  readonly userStats = computed<DashboardUserStats>(() => {
    return (
      this.dashboard()?.users ?? {
        total: 0,
        active: 0,
        suspended: 0,
        by_role: { BUYER: 0, SELLER: 0, ADMIN: 0 },
      }
    );
  });

  readonly categoryStats = computed<DashboardCategoryStats>(() => {
    return this.dashboard()?.categories ?? { total: 0, active: 0 };
  });

  readonly recentAuditLogs = computed<AuditLog[]>(() => {
    return (this.dashboard()?.recent_activity ?? []).slice(0, 5);
  });

  ngOnInit(): void {
    this.refreshAll();
  }

  refreshAll(): void {
    this.isLoading.set(true);
    this.loadError.set('');
    this.liveAnnouncement.set('');

    this.adminService
      .getDashboardSummary()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (summary) => {
          this.dashboard.set(summary ?? null);
          this.isLoading.set(false);
          this.liveAnnouncement.set('Dashboard updated');
        },
        error: (err: unknown) => {
          this.loadError.set(
            this.extractErrorMessage(err, 'Failed to load dashboard summary.'),
          );
          this.isLoading.set(false);
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
