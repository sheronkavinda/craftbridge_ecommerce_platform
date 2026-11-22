import { Component, inject } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { AccountApiService } from '../account-api.service';

@Component({
  standalone: true,
  selector: 'app-forgot-password',
  imports: [ReactiveFormsModule, RouterLink],
  templateUrl: './forgot-password.component.html',
})
export class ForgotPasswordComponent {
  private readonly fb = inject(FormBuilder);
  private readonly api = inject(AccountApiService);
  message = '';
  error = '';
  loading = false;
  form = this.fb.nonNullable.group({ email: ['', [Validators.required, Validators.email]] });

  submit(): void {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    this.loading = true;
    this.error = '';
    this.api.forgotPassword(this.form.controls.email.value).subscribe({
      next: (response) => {
        this.message = response.message;
        this.loading = false;
      },
      error: () => {
        this.error = 'We could not process that request. Please try again.';
        this.loading = false;
      },
    });
  }
}
