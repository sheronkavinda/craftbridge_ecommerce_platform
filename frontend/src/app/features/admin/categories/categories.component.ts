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
  inject,
  signal,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { Category, CategoryPayload } from '../admin.models';
import { AdminService } from '../admin.service';

@Component({
  selector: 'app-admin-categories',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule],
  templateUrl: './categories.component.html',
  styleUrl: './categories.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class CategoriesComponent implements OnInit {
  private readonly adminService = inject(AdminService);
  private readonly fb = inject(FormBuilder);
  private readonly destroyRef = inject(DestroyRef);

  @ViewChild('nameInput') nameInputRef?: ElementRef<HTMLInputElement>;
  @ViewChild('confirmDeleteBtn') confirmDeleteBtnRef?: ElementRef<HTMLButtonElement>;

  private lastFocusedElement: HTMLElement | null = null;

  readonly categories = signal<Category[]>([]);
  readonly isLoading = signal<boolean>(false);
  readonly loadError = signal<string>('');

  readonly isFormOpen = signal<boolean>(false);
  readonly editingCategoryId = signal<string | null>(null);
  readonly isSaving = signal<boolean>(false);
  readonly formError = signal<string>('');
  readonly successMessage = signal<string>('');

  readonly pendingDeleteCategory = signal<Category | null>(null);
  readonly isDeleting = signal<boolean>(false);
  readonly deleteError = signal<string>('');

  slugManuallyEdited = false;

  readonly categoryForm = this.fb.nonNullable.group({
    name: ['', [Validators.required, Validators.minLength(2), Validators.maxLength(120)]],
    slug: [
      '',
      [
        Validators.required,
        Validators.minLength(2),
        Validators.maxLength(120),
        Validators.pattern(/^[a-z0-9-]+$/),
      ],
    ],
    description: ['', [Validators.maxLength(512)]],
    is_active: [true],
  });

  ngOnInit(): void {
    this.categoryForm.controls.name.valueChanges
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((nameValue) => {
        if (!this.slugManuallyEdited) {
          this.categoryForm.controls.slug.setValue(this.generateSlug(nameValue), {
            emitEvent: false,
          });
          this.categoryForm.controls.slug.updateValueAndValidity({ emitEvent: false });
        }
      });

    this.loadCategories();
  }

  @HostListener('document:keydown.escape')
  onEscapeKey(): void {
    if (this.pendingDeleteCategory() && !this.isDeleting()) {
      this.cancelDelete();
      return;
    }
    if (this.isFormOpen() && !this.isSaving()) {
      this.closeForm();
    }
  }

  loadCategories(): void {
    this.isLoading.set(true);
    this.loadError.set('');

    this.adminService
      .listCategories()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (items) => {
          this.categories.set(items ?? []);
          this.isLoading.set(false);
        },
        error: (err: unknown) => {
          this.loadError.set(this.extractErrorMessage(err, 'Failed to load categories.'));
          this.isLoading.set(false);
        },
      });
  }

  openCreateForm(trigger?: EventTarget | null): void {
    this.captureTriggerFocus(trigger);
    this.editingCategoryId.set(null);
    this.slugManuallyEdited = false;
    this.formError.set('');
    this.successMessage.set('');
    this.categoryForm.reset({
      name: '',
      slug: '',
      description: '',
      is_active: true,
    });
    this.isFormOpen.set(true);
    this.focusDialogElement(() => this.nameInputRef?.nativeElement);
  }

  openEditForm(category: Category, trigger?: EventTarget | null): void {
    this.captureTriggerFocus(trigger);
    this.editingCategoryId.set(category.id);
    this.slugManuallyEdited = true;
    this.formError.set('');
    this.successMessage.set('');
    this.categoryForm.reset({
      name: category.name,
      slug: category.slug,
      description: category.description ?? '',
      is_active: category.is_active,
    });
    this.isFormOpen.set(true);
    this.focusDialogElement(() => this.nameInputRef?.nativeElement);
  }

  closeForm(): void {
    this.isFormOpen.set(false);
    this.editingCategoryId.set(null);
    this.formError.set('');
    this.restoreTriggerFocus();
  }

  onSlugManualEdit(): void {
    const currentSlug = this.categoryForm.controls.slug.value;
    this.slugManuallyEdited = currentSlug.trim().length > 0;
  }

  generateSlug(value: string): string {
    return (value ?? '')
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 120);
  }

  saveCategory(): void {
    if (this.categoryForm.invalid || this.isSaving()) {
      this.categoryForm.markAllAsTouched();
      return;
    }

    const raw = this.categoryForm.getRawValue();
    const payload: CategoryPayload = {
      name: raw.name.trim(),
      slug: raw.slug.trim(),
      description: raw.description.trim(),
      is_active: raw.is_active,
    };

    this.isSaving.set(true);
    this.formError.set('');
    this.successMessage.set('');

    const editId = this.editingCategoryId();
    const request$ = editId
      ? this.adminService.updateCategory(editId, payload)
      : this.adminService.createCategory(payload);

    request$.pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: () => {
        this.isSaving.set(false);
        this.closeForm();
        this.successMessage.set(
          editId ? 'Category updated successfully.' : 'Category created successfully.',
        );
        this.loadCategories();
      },
      error: (err: unknown) => {
        this.isSaving.set(false);
        this.formError.set(this.extractErrorMessage(err, 'Failed to save category.'));
      },
    });
  }

  requestDelete(category: Category, trigger?: EventTarget | null): void {
    this.captureTriggerFocus(trigger);
    this.pendingDeleteCategory.set(category);
    this.deleteError.set('');
    this.successMessage.set('');
    this.focusDialogElement(() => this.confirmDeleteBtnRef?.nativeElement);
  }

  cancelDelete(): void {
    this.pendingDeleteCategory.set(null);
    this.deleteError.set('');
    this.restoreTriggerFocus();
  }

  confirmDelete(): void {
    const target = this.pendingDeleteCategory();
    if (!target || this.isDeleting()) {
      return;
    }

    this.isDeleting.set(true);
    this.deleteError.set('');
    this.successMessage.set('');

    this.adminService
      .deleteCategory(target.id)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: () => {
          this.isDeleting.set(false);
          this.pendingDeleteCategory.set(null);
          this.restoreTriggerFocus();
          this.successMessage.set(`Category "${target.name}" deleted.`);
          this.loadCategories();
        },
        error: (err: unknown) => {
          this.isDeleting.set(false);
          this.deleteError.set(this.extractErrorMessage(err, 'Failed to delete category.'));
        },
      });
  }

  hasFieldError(controlName: 'name' | 'slug' | 'description'): boolean {
    const ctrl = this.categoryForm.controls[controlName];
    return ctrl.invalid && (ctrl.touched || ctrl.dirty);
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
