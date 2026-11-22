import { Component, inject, OnInit } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { AccountApiService } from '../account-api.service';
import { AuthService } from '../../../core/auth/auth.service';

@Component({
  standalone: true,
  selector: 'app-profile',
  imports: [ReactiveFormsModule, RouterLink],
  templateUrl: './profile.component.html',
})
export class ProfileComponent implements OnInit {
  private readonly fb = inject(FormBuilder);
  private readonly api = inject(AccountApiService);
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  message = '';
  error = '';
  loading = true;
  saving = false;
  form = this.fb.nonNullable.group({ name: ['', Validators.required], email: [''] });

  ngOnInit(): void {
    this.api.me().subscribe({
      next: (user) => {
        this.form.patchValue({ name: user.name, email: user.email });
        this.form.controls.email.disable();
        this.loading = false;
      },
      error: () => {
        this.error = 'Your profile could not be loaded. Please try again.';
        this.loading = false;
      },
    });
  }

  submit(): void {
    if (this.form.invalid) return;
    this.saving = true;
    this.error = '';
    this.api.updateProfile(this.form.controls.name.value).subscribe({
      next: (response) => {
        this.message = response.message;
        this.saving = false;
      },
      error: () => {
        this.error = 'Your changes could not be saved.';
        this.saving = false;
      },
    });
  }

  logout(): void {
    this.auth.logout().subscribe({
      next: () => this.router.navigate(['/login']),
      error: () => {
        void this.router.navigate(['/login'], { queryParams: { reason: 'logout-warning' } });
      },
    });
  }
}
