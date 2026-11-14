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
import { AuditLog } from '../admin.models';
import { AdminService } from '../admin.service';

@Component({
  selector: 'app-admin-audit-log',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './audit-log.component.html',
  styleUrl: './audit-log.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class AuditLogComponent implements OnInit {
  private readonly adminService = inject(AdminService);
  private readonly destroyRef = inject(DestroyRef);

  readonly pageSize = 25;

  readonly logs = signal<AuditLog[]>([]);
  readonly isLoading = signal<boolean>(false);
  readonly loadError = signal<string>('');
  readonly liveAnnouncement = signal<string>('');

  readonly searchQuery = signal<string>('');
  readonly selectedAction = signal<string>('');
  readonly selectedOutcome = signal<string>('');
  readonly currentPage = signal<number>(1);

  private readonly discoveredActions = signal<string[]>([]);
  readonly expandedIds = signal<ReadonlySet<number>>(new Set<number>());
  readonly copiedEntryId = signal<number | null>(null);

  private copyTimeoutId: ReturnType<typeof setTimeout> | null = null;

  readonly offset = computed<number>(() => (this.currentPage() - 1) * this.pageSize);

  readonly distinctActions = computed<string[]>(() => {
    const set = new Set<string>(this.discoveredActions());
    for (const entry of this.logs()) {
      const act = (entry.action ?? '').trim().toUpperCase();
      if (act) {
        set.add(act);
      }
    }
    const current = this.selectedAction().trim().toUpperCase();
    if (current) {
      set.add(current);
    }
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  });

  readonly filteredLogs = computed<AuditLog[]>(() => {
    const query = this.searchQuery().trim().toLowerCase();
    const actionFilter = this.selectedAction().trim().toUpperCase();
    const outcomeFilter = this.selectedOutcome().trim().toUpperCase();

    return this.logs().filter((entry) => {
      if (actionFilter && (entry.action ?? '').trim().toUpperCase() !== actionFilter) {
        return false;
      }
      if (outcomeFilter && (entry.outcome ?? '').trim().toUpperCase() !== outcomeFilter) {
        return false;
      }
      if (!query) {
        return true;
      }

      const actorName = (entry.actor_name ?? '').toLowerCase();
      const actorId = (entry.actor_id ?? '').toLowerCase();
      const rawAction = (entry.action ?? '').toLowerCase();
      const readableAction = this.formatActionLabel(entry.action).toLowerCase();
      const target = (entry.target ?? '').toLowerCase();
      const details = (entry.details ?? '').toLowerCase();
      const requestId = (entry.request_id ?? '').toLowerCase();

      return (
        actorName.includes(query) ||
        actorId.includes(query) ||
        rawAction.includes(query) ||
        readableAction.includes(query) ||
        target.includes(query) ||
        details.includes(query) ||
        requestId.includes(query)
      );
    });
  });

  readonly hasActiveFilters = computed<boolean>(() => {
    return Boolean(
      this.searchQuery().trim() ||
        this.selectedAction() ||
        this.selectedOutcome(),
    );
  });

  readonly canGoPrevious = computed<boolean>(() => !this.isLoading() && this.currentPage() > 1);

  readonly canGoNext = computed<boolean>(
    () => !this.isLoading() && this.logs().length >= this.pageSize,
  );

  readonly showingRangeText = computed<string>(() => {
    const count = this.filteredLogs().length;
    if (count === 0) {
      return 'Showing 0–0';
    }
    const start = this.offset() + 1;
    const end = this.offset() + count;
    return `Showing ${start}–${end}`;
  });

  ngOnInit(): void {
    this.loadLogs();
  }

  loadLogs(): void {
    this.isLoading.set(true);
    this.loadError.set('');

    this.adminService
      .listAuditLogs(
        '',
        this.selectedAction(),
        this.selectedOutcome(),
        this.pageSize,
        this.offset(),
      )
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (items) => {
          const list = items ?? [];
          this.logs.set(list);
          this.recordDiscoveredActions(list);
          this.isLoading.set(false);
          this.liveAnnouncement.set(
            `Loaded ${list.length} ${list.length === 1 ? 'entry' : 'entries'}`,
          );
        },
        error: (err: unknown) => {
          this.loadError.set(this.extractErrorMessage(err, 'Failed to load audit logs.'));
          this.isLoading.set(false);
        },
      });
  }

  refreshLogs(): void {
    this.loadLogs();
  }

  onSearchChange(query: string): void {
    this.searchQuery.set(query);
  }

  onActionChange(action: string): void {
    this.selectedAction.set(action);
    this.currentPage.set(1);
    this.loadLogs();
  }

  onOutcomeChange(outcome: string): void {
    this.selectedOutcome.set(outcome);
    this.currentPage.set(1);
    this.loadLogs();
  }

  resetFilters(): void {
    const needsRefetch = Boolean(
      this.selectedAction() || this.selectedOutcome() || this.currentPage() !== 1,
    );
    this.searchQuery.set('');
    this.selectedAction.set('');
    this.selectedOutcome.set('');
    this.currentPage.set(1);
    if (needsRefetch) {
      this.loadLogs();
    }
  }

  goToPreviousPage(): void {
    if (!this.canGoPrevious()) {
      return;
    }
    this.currentPage.update((page) => Math.max(1, page - 1));
    this.loadLogs();
  }

  goToNextPage(): void {
    if (!this.canGoNext()) {
      return;
    }
    this.currentPage.update((page) => page + 1);
    this.loadLogs();
  }

  formatActionLabel(action: string): string {
    const raw = (action ?? '').trim();
    if (!raw) {
      return 'Unknown action';
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

  shouldTruncateDetails(details: string): boolean {
    return (details ?? '').trim().length > 90;
  }

  isExpanded(entryId: number): boolean {
    return this.expandedIds().has(entryId);
  }

  toggleDetails(entryId: number): void {
    this.expandedIds.update((prev) => {
      const next = new Set(prev);
      if (next.has(entryId)) {
        next.delete(entryId);
      } else {
        next.add(entryId);
      }
      return next;
    });
  }

  copyRequestId(entry: AuditLog): void {
    const reqId = (entry.request_id ?? '').trim();
    if (!reqId) {
      return;
    }

    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      void navigator.clipboard.writeText(reqId).catch(() => {
        // Fallback no-op if clipboard permission is unavailable
      });
    }

    this.copiedEntryId.set(entry.id);
    if (this.copyTimeoutId) {
      clearTimeout(this.copyTimeoutId);
    }
    this.copyTimeoutId = setTimeout(() => {
      if (this.copiedEntryId() === entry.id) {
        this.copiedEntryId.set(null);
      }
    }, 2000);
  }

  private recordDiscoveredActions(items: AuditLog[]): void {
    const next = new Set<string>(this.discoveredActions());
    for (const item of items) {
      const act = (item.action ?? '').trim().toUpperCase();
      if (act) {
        next.add(act);
      }
    }
    this.discoveredActions.set(Array.from(next));
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
