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
import { SellerApplication } from '../admin.models';
import { AdminService } from '../admin.service';

export type SellerFilterTab = 'PENDING' | 'APPROVED' | 'REJECTED' | 'ALL';

@Component({
  selector: 'app-admin-seller-approvals',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule],
  templateUrl: './seller-approvals.component.html',
  styleUrl: './seller-approvals.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SellerApprovalsComponent implements OnInit {
  private readonly adminService = inject(AdminService);
  private readonly fb = inject(FormBuilder);
  private readonly destroyRef = inject(DestroyRef);

  @ViewChild('confirmApproveBtn') confirmApproveBtnRef?: ElementRef<HTMLButtonElement>;
  @ViewChild('rejectNotesInput') rejectNotesInputRef?: ElementRef<HTMLTextAreaElement>;

  private lastFocusedElement: HTMLElement | null = null;

  readonly applications = signal<SellerApplication[]>([]);
  readonly isLoading = signal<boolean>(false);
  readonly loadError = signal<string>('');

  readonly activeTab = signal<SellerFilterTab>('PENDING');

  readonly pendingApproveTarget = signal<SellerApplication | null>(null);
  readonly pendingRejectTarget = signal<SellerApplication | null>(null);
  readonly isSubmitting = signal<boolean>(false);
  readonly dialogError = signal<string>('');
  readonly successMessage = signal<string>('');

  readonly approveForm = this.fb.nonNullable.group({
    notes: ['', [Validators.maxLength(512)]],
  });

  readonly rejectForm = this.fb.nonNullable.group({
    notes: ['', [Validators.required, Validators.minLength(3), Validators.maxLength(512)]],
  });

  readonly counts = computed(() => {
    const all = this.applications();
    let pending = 0;
    let approved = 0;
    let rejected = 0;
    for (const app of all) {
      const st = app.status.trim().toUpperCase();
      if (st === 'PENDING') pending++;
      else if (st === 'APPROVED') approved++;
      else if (st === 'REJECTED') rejected++;
    }
    return {
      PENDING: pending,
      APPROVED: approved,
      REJECTED: rejected,
      ALL: all.length,
    };
  });

  readonly filteredApplications = computed<SellerApplication[]>(() => {
    const tab = this.activeTab();
    const all = this.applications();
    if (tab === 'ALL') {
      return all;
    }
    return all.filter((app) => app.status.trim().toUpperCase() === tab);
  });

  readonly emptyStateMessage = computed<string>(() => {
    switch (this.activeTab()) {
      case 'PENDING':
        return 'No pending applications';
      case 'APPROVED':
        return 'No approved applications';
      case 'REJECTED':
        return 'No rejected applications';
      default:
        return 'No seller applications found';
    }
  });

  ngOnInit(): void {
    this.loadApplications();
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

  loadApplications(): void {
    this.isLoading.set(true);
    this.loadError.set('');

    this.adminService
      .listSellerApplications('', 200, 0)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (items) => {
          this.applications.set(items ?? []);
          this.isLoading.set(false);
        },
        error: (err: unknown) => {
          this.loadError.set(
            this.extractErrorMessage(err, 'Failed to load seller applications.'),
          );
          this.isLoading.set(false);
        },
      });
  }

  selectTab(tab: SellerFilterTab): void {
    this.activeTab.set(tab);
  }

  isPending(app: SellerApplication): boolean {
    return app.status.trim().toUpperCase() === 'PENDING';
  }

  openApproveDialog(app: SellerApplication, trigger?: EventTarget | null): void {
    if (!this.isPending(app)) {
      return;
    }
    this.captureTriggerFocus(trigger);
    this.dialogError.set('');
    this.successMessage.set('');
    this.approveForm.reset({ notes: app.notes ?? '' });
    this.pendingApproveTarget.set(app);
    this.focusDialogElement(() => this.confirmApproveBtnRef?.nativeElement);
  }

  closeApproveDialog(): void {
    this.pendingApproveTarget.set(null);
    this.dialogError.set('');
    this.restoreTriggerFocus();
  }

  confirmApprove(): void {
    const target = this.pendingApproveTarget();
    if (!target || this.isSubmitting() || this.approveForm.invalid) {
      return;
    }

    const notes = this.approveForm.controls.notes.value.trim();
    this.isSubmitting.set(true);
    this.dialogError.set('');
    this.successMessage.set('');

    this.adminService
      .updateSellerStatus(target.id, {
        status: 'APPROVED',
        ...(notes ? { notes } : {}),
      })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (updated) => {
          this.applyUpdatedApplication(updated);
          this.isSubmitting.set(false);
          this.pendingApproveTarget.set(null);
          this.restoreTriggerFocus();
          this.successMessage.set(
            `Storefront "${updated.store_name}" has been approved.`,
          );
        },
        error: (err: unknown) => {
          this.isSubmitting.set(false);
          this.dialogError.set(
            this.extractErrorMessage(err, 'Failed to approve seller application.'),
          );
        },
      });
  }

  openRejectDialog(app: SellerApplication, trigger?: EventTarget | null): void {
    if (!this.isPending(app)) {
      return;
    }
    this.captureTriggerFocus(trigger);
    this.dialogError.set('');
    this.successMessage.set('');
    this.rejectForm.reset({ notes: '' });
    this.pendingRejectTarget.set(app);
    this.focusDialogElement(() => this.rejectNotesInputRef?.nativeElement);
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

    const notes = this.rejectForm.controls.notes.value.trim();
    if (this.rejectForm.invalid || notes.length < 3) {
      this.rejectForm.markAllAsTouched();
      return;
    }

    this.isSubmitting.set(true);
    this.dialogError.set('');
    this.successMessage.set('');

    this.adminService
      .updateSellerStatus(target.id, {
        status: 'REJECTED',
        notes,
      })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (updated) => {
          this.applyUpdatedApplication(updated);
          this.isSubmitting.set(false);
          this.pendingRejectTarget.set(null);
          this.restoreTriggerFocus();
          this.successMessage.set(
            `Storefront "${updated.store_name}" has been rejected.`,
          );
        },
        error: (err: unknown) => {
          this.isSubmitting.set(false);
          this.dialogError.set(
            this.extractErrorMessage(err, 'Failed to reject seller application.'),
          );
        },
      });
  }

  hasRejectNotesError(): boolean {
    const ctrl = this.rejectForm.controls.notes;
    return ctrl.invalid && (ctrl.touched || ctrl.dirty);
  }

  private applyUpdatedApplication(updated: SellerApplication): void {
    this.applications.update((list) =>
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
