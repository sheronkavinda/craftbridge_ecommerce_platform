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
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { RefundRecord } from '../admin.models';
import { AdminService } from '../admin.service';

export type RefundFilterTab = 'PENDING' | 'APPROVED' | 'REJECTED' | 'ALL';

@Component({
  selector: 'app-admin-refunds',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule],
  templateUrl: './refunds.component.html',
  styleUrl: './refunds.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class RefundsComponent implements OnInit {
  private readonly adminService = inject(AdminService);
  private readonly fb = inject(FormBuilder);
  private readonly destroyRef = inject(DestroyRef);

  @ViewChild('confirmApproveBtn') confirmApproveBtnRef?: ElementRef<HTMLButtonElement>;
  @ViewChild('rejectReasonInput') rejectReasonInputRef?: ElementRef<HTMLTextAreaElement>;

  private lastFocusedElement: HTMLElement | null = null;

  readonly refunds = signal<RefundRecord[]>([]);
  readonly isLoading = signal<boolean>(false);
  readonly loadError = signal<string>('');

  readonly activeTab = signal<RefundFilterTab>('PENDING');

  readonly pendingApproveTarget = signal<RefundRecord | null>(null);
  readonly pendingRejectTarget = signal<RefundRecord | null>(null);
  readonly isSubmitting = signal<boolean>(false);
  readonly dialogError = signal<string>('');
  readonly successMessage = signal<string>('');

  readonly rejectForm = this.fb.nonNullable.group({
    reason: ['', [Validators.required, Validators.minLength(3), Validators.maxLength(512)]],
  });

  readonly counts = computed(() => {
    const all = this.refunds();
    let pending = 0;
    let approved = 0;
    let rejected = 0;
    for (const r of all) {
      const st = r.status.trim().toUpperCase();
      if (st === 'PENDING') {
        pending++;
      } else if (st === 'APPROVED' || st === 'PROCESSED' || st === 'COMPLETED') {
        approved++;
      } else if (st === 'REJECTED') {
        rejected++;
      }
    }
    return {
      PENDING: pending,
      APPROVED: approved,
      REJECTED: rejected,
      ALL: all.length,
    };
  });

  readonly pendingSummary = computed(() => {
    const all = this.refunds();
    let count = 0;
    let totalAmount = 0;
    for (const r of all) {
      if (r.status.trim().toUpperCase() === 'PENDING') {
        count++;
        totalAmount += Number(r.amount) || 0;
      }
    }
    return { count, totalAmount };
  });

  readonly filteredRefunds = computed<RefundRecord[]>(() => {
    const tab = this.activeTab();
    const all = this.refunds();
    if (tab === 'ALL') {
      return all;
    }
    return all.filter((r) => {
      const st = r.status.trim().toUpperCase();
      if (tab === 'APPROVED') {
        return st === 'APPROVED' || st === 'PROCESSED' || st === 'COMPLETED';
      }
      return st === tab;
    });
  });

  readonly emptyStateMessage = computed<string>(() => {
    switch (this.activeTab()) {
      case 'PENDING':
        return 'No pending refund requests';
      case 'APPROVED':
        return 'No approved refunds';
      case 'REJECTED':
        return 'No rejected refunds';
      default:
        return 'No refund records found';
    }
  });

  ngOnInit(): void {
    this.loadRefunds();
  }

  @HostListener('document:keydown.escape')
  onEscapeKey(): void {
    if (this.isSubmitting()) {
      return;
    }
    if (this.pendingApproveTarget()) {
      this.closeApproveDialog();
      return;
    }
    if (this.pendingRejectTarget()) {
      this.closeRejectDialog();
    }
  }

  loadRefunds(): void {
    this.isLoading.set(true);
    this.loadError.set('');

    this.adminService
      .listRefunds('', 200, 0)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (items) => {
          this.refunds.set(items ?? []);
          this.isLoading.set(false);
        },
        error: (err: unknown) => {
          this.loadError.set(this.extractErrorMessage(err, 'Failed to load refunds.'));
          this.isLoading.set(false);
        },
      });
  }

  selectTab(tab: RefundFilterTab): void {
    this.activeTab.set(tab);
  }

  isPending(refund: RefundRecord): boolean {
    return refund.status.trim().toUpperCase() === 'PENDING';
  }

  isApprovedGroup(status: string): boolean {
    const st = status.trim().toUpperCase();
    return st === 'APPROVED' || st === 'PROCESSED' || st === 'COMPLETED';
  }

  formatLkr(amount: number): string {
    const numeric = Number(amount) || 0;
    return (
      'LKR ' +
      numeric.toLocaleString('en-US', {
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
      })
    );
  }

  openApproveDialog(refund: RefundRecord, trigger?: EventTarget | null): void {
    if (!this.isPending(refund)) {
      return;
    }
    this.captureTriggerFocus(trigger);
    this.dialogError.set('');
    this.successMessage.set('');
    this.pendingApproveTarget.set(refund);
    this.focusDialogElement(() => this.confirmApproveBtnRef?.nativeElement);
  }

  closeApproveDialog(): void {
    this.pendingApproveTarget.set(null);
    this.dialogError.set('');
    this.restoreTriggerFocus();
  }

  confirmApprove(): void {
    const target = this.pendingApproveTarget();
    if (!target || this.isSubmitting()) {
      return;
    }

    this.isSubmitting.set(true);
    this.dialogError.set('');
    this.successMessage.set('');

    this.adminService
      .updateRefundStatus(target.id, { status: 'APPROVED' })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (updated) => {
          this.applyUpdatedRefund(updated);
          this.isSubmitting.set(false);
          this.pendingApproveTarget.set(null);
          this.restoreTriggerFocus();
          this.successMessage.set(
            `Refund ${updated.id} (${this.formatLkr(updated.amount)}) has been approved.`,
          );
        },
        error: (err: unknown) => {
          this.isSubmitting.set(false);
          this.dialogError.set(this.extractErrorMessage(err, 'Failed to approve refund.'));
        },
      });
  }

  openRejectDialog(refund: RefundRecord, trigger?: EventTarget | null): void {
    if (!this.isPending(refund)) {
      return;
    }
    this.captureTriggerFocus(trigger);
    this.dialogError.set('');
    this.successMessage.set('');
    this.rejectForm.reset({ reason: '' });
    this.pendingRejectTarget.set(refund);
    this.focusDialogElement(() => this.rejectReasonInputRef?.nativeElement);
  }

  closeRejectDialog(): void {
    this.pendingRejectTarget.set(null);
    this.dialogError.set('');
    this.restoreTriggerFocus();
  }

  confirmReject(): void {
    const target = this.pendingRejectTarget();
    if (!target || this.isSubmitting()) {
      return;
    }

    const reason = this.rejectForm.controls.reason.value.trim();
    if (this.rejectForm.invalid || reason.length < 3) {
      this.rejectForm.markAllAsTouched();
      return;
    }

    this.isSubmitting.set(true);
    this.dialogError.set('');
    this.successMessage.set('');

    this.adminService
      .updateRefundStatus(target.id, {
        status: 'REJECTED',
        reason,
      })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (updated) => {
          this.applyUpdatedRefund(updated);
          this.isSubmitting.set(false);
          this.pendingRejectTarget.set(null);
          this.restoreTriggerFocus();
          this.successMessage.set(`Refund ${updated.id} has been rejected.`);
        },
        error: (err: unknown) => {
          this.isSubmitting.set(false);
          this.dialogError.set(this.extractErrorMessage(err, 'Failed to reject refund.'));
        },
      });
  }

  hasRejectReasonError(): boolean {
    const ctrl = this.rejectForm.controls.reason;
    return ctrl.invalid && (ctrl.touched || ctrl.dirty);
  }

  private applyUpdatedRefund(updated: RefundRecord): void {
    this.refunds.update((list) =>
      list.map((item) => (item.id === updated.id ? updated : item)),
    );
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
