import { CommonModule } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  ElementRef,
  HostListener,
  OnInit,
  ViewChild,
  computed,
  inject,
  signal,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { environment } from '../../../../environments/environment';
import { UserSummary } from '../admin.models';
import { AdminService } from '../admin.service';

@Component({
  selector: 'app-admin-users',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './users.component.html',
  styleUrl: './users.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class UsersComponent implements OnInit {
  private readonly adminService = inject(AdminService);
  private readonly destroyRef = inject(DestroyRef);

  @ViewChild('confirmSuspendBtn') confirmSuspendBtnRef?: ElementRef<HTMLButtonElement>;

  private lastFocusedElement: HTMLElement | null = null;

  readonly users = signal<UserSummary[]>([]);
  readonly isLoading = signal<boolean>(false);
  readonly loadError = signal<string>('');

  readonly selectedRole = signal<string>('');
  readonly selectedStatus = signal<string>('');
  readonly searchQuery = signal<string>('');

  readonly pendingSuspendUser = signal<UserSummary | null>(null);
  readonly suspendReason = signal<string>('');
  readonly updatingUserId = signal<string | null>(null);

  readonly successMessage = signal<string>('');
  readonly actionError = signal<string>('');

  readonly filteredUsers = computed<UserSummary[]>(() => {
    const query = this.searchQuery().trim().toLowerCase();
    const list = this.users();
    if (!query) {
      return list;
    }
    return list.filter(
      (u) =>
        u.name.toLowerCase().includes(query) ||
        u.email.toLowerCase().includes(query),
    );
  });

  readonly hasActiveFilters = computed<boolean>(() => {
    return Boolean(
      this.selectedRole() ||
        this.selectedStatus() ||
        this.searchQuery().trim(),
    );
  });

  ngOnInit(): void {
    this.loadUsers();
  }

  @HostListener('document:keydown.escape')
  onEscapeKey(): void {
    if (this.pendingSuspendUser() && !this.updatingUserId()) {
      this.cancelSuspend();
    }
  }

  loadUsers(): void {
    this.isLoading.set(true);
    this.loadError.set('');

    this.adminService
      .listUsers(this.selectedRole(), this.selectedStatus())
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (items) => {
          this.users.set(items ?? []);
          this.isLoading.set(false);
        },
        error: (err: unknown) => {
          this.loadError.set(this.extractErrorMessage(err, 'Failed to load users.'));
          this.isLoading.set(false);
        },
      });
  }

  onRoleChange(role: string): void {
    this.selectedRole.set(role);
    this.loadUsers();
  }

  onStatusChange(status: string): void {
    this.selectedStatus.set(status);
    this.loadUsers();
  }

  onSearchChange(query: string): void {
    this.searchQuery.set(query);
  }

  clearFilters(): void {
    const needsRefetch = Boolean(this.selectedRole() || this.selectedStatus());
    this.selectedRole.set('');
    this.selectedStatus.set('');
    this.searchQuery.set('');
    if (needsRefetch) {
      this.loadUsers();
    }
  }

  isProtectedUser(user: UserSummary): boolean {
    if (user.role.trim().toUpperCase() === 'ADMIN') {
      return true;
    }
    const actorId = this.getCurrentActorId();
    return Boolean(actorId && user.id.trim().toLowerCase() === actorId.toLowerCase());
  }

  isSuspended(user: UserSummary): boolean {
    const status = user.status.trim().toUpperCase();
    return status === 'SUSPENDED' || status === 'BANNED';
  }

  requestSuspend(user: UserSummary, trigger?: EventTarget | null): void {
    if (this.isProtectedUser(user)) {
      return;
    }
    this.captureTriggerFocus(trigger);
    this.pendingSuspendUser.set(user);
    this.suspendReason.set('');
    this.actionError.set('');
    this.successMessage.set('');
    this.focusDialogElement(() => this.confirmSuspendBtnRef?.nativeElement);
  }

  cancelSuspend(): void {
    this.pendingSuspendUser.set(null);
    this.suspendReason.set('');
    this.restoreTriggerFocus();
  }

  confirmSuspend(): void {
    const target = this.pendingSuspendUser();
    if (!target || this.updatingUserId()) {
      return;
    }

    const reason = this.suspendReason().trim();
    this.updatingUserId.set(target.id);
    this.actionError.set('');
    this.successMessage.set('');

    this.adminService
      .updateUserStatus(target.id, {
        status: 'SUSPENDED',
        ...(reason ? { reason } : {}),
      })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (updated) => {
          this.applyUpdatedUser(updated);
          this.updatingUserId.set(null);
          this.pendingSuspendUser.set(null);
          this.suspendReason.set('');
          this.restoreTriggerFocus();
          this.successMessage.set(`User "${updated.name}" has been suspended.`);
        },
        error: (err: unknown) => {
          this.updatingUserId.set(null);
          this.pendingSuspendUser.set(null);
          this.restoreTriggerFocus();
          this.actionError.set(this.extractErrorMessage(err, 'Failed to suspend user.'));
        },
      });
  }

  activateUser(user: UserSummary): void {
    if (this.isProtectedUser(user) || this.updatingUserId()) {
      return;
    }

    this.updatingUserId.set(user.id);
    this.actionError.set('');
    this.successMessage.set('');

    this.adminService
      .updateUserStatus(user.id, { status: 'ACTIVE' })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (updated) => {
          this.applyUpdatedUser(updated);
          this.updatingUserId.set(null);
          this.successMessage.set(`User "${updated.name}" has been activated.`);
        },
        error: (err: unknown) => {
          this.updatingUserId.set(null);
          this.actionError.set(this.extractErrorMessage(err, 'Failed to activate user.'));
        },
      });
  }

  private applyUpdatedUser(updated: UserSummary): void {
    const activeStatusFilter = this.selectedStatus().trim().toUpperCase();
    this.users.update((list) => {
      if (activeStatusFilter && updated.status.trim().toUpperCase() !== activeStatusFilter) {
        return list.filter((u) => u.id !== updated.id);
      }
      return list.map((u) => (u.id === updated.id ? updated : u));
    });
  }

  private getCurrentActorId(): string {
    if (typeof localStorage === 'undefined') {
      return !environment.production ? 'admin-001' : '';
    }
    const stored = (localStorage.getItem('user_id') ?? '').trim();
    return stored || (!environment.production ? 'admin-001' : '');
  }

  private captureTriggerFocus(trigger?: EventTarget | null): void {
    if (trigger instanceof HTMLElement) {
      this.lastFocusedElement = trigger;
    } else if (typeof document !== 'undefined' && document.activeElement instanceof HTMLElement) {
      this.lastFocusedElement = document.activeElement;
    }
  }

  private focusDialogElement(getter: () => HTMLElement | undefined): void {
    setTimeout(() => {
      getter()?.focus();
    }, 0);
  }

  private restoreTriggerFocus(): void {
    const el = this.lastFocusedElement;
    this.lastFocusedElement = null;
    if (el) {
      setTimeout(() => {
        el.focus();
      }, 0);
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
