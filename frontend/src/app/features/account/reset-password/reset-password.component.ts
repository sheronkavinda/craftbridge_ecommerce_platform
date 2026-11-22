import { Component, inject } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { AccountApiService } from '../account-api.service';

@Component({
  standalone: true,
  selector: 'app-reset-password',
  imports: [ReactiveFormsModule],
  templateUrl: './reset-password.component.html',
})
export class ResetPasswordComponent {
  private readonly fb = inject(FormBuilder);
  private readonly api = inject(AccountApiService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  message = '';
  loading = false;
  private readonly token = this.route.snapshot.queryParamMap.get('token') ?? '';
  form = this.fb.nonNullable.group({
    password: ['', [Validators.required, Validators.minLength(8), Validators.maxLength(72)]],
  });

  submit(): void {
    if (!this.token) {
      this.message = 'This reset link is invalid or expired.';
      return;
    }
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    this.loading = true;
    this.message = '';
    this.api.resetPassword(this.token, this.form.controls.password.value).subscribe({
      next: () => this.router.navigate(['/login']),
      error: (err) => {
        this.message = err.error?.error ?? 'Unable to reset password';
        this.loading = false;
      },
    });
  }
}
