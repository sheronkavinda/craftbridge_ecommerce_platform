import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { environment } from '../../../environments/environment';
import { Header } from '../../shared/components/header/header';

interface SellerApplication {
  store_name: string;
  country_code: string;
  status: 'pending' | 'approved' | 'rejected';
  decision_reason: string | null;
}

interface ApplicationResponse {
  application: SellerApplication;
}

@Component({
  standalone: true,
  selector: 'app-seller-application',
  imports: [FormsModule, Header, RouterLink],
  template: `
    <app-header />
    <main class="min-h-screen bg-[#faf8f5] px-5 py-10 text-[#14213a]">
      <section class="mx-auto max-w-2xl rounded-xl border border-stone-200 bg-white p-6 sm:p-8">
        <h1 class="mb-6 font-serif text-3xl font-bold">Seller application</h1>

        @if (message()) {
          <p role="alert" class="mb-4 text-red-700">{{ message() }}</p>
        }

        @if (state() === 'loading') {
          <p role="status">Loading your application…</p>
        } @else if (state() === 'error') {
          <button type="button" class="cursor-pointer underline" (click)="load()">Try again</button>
        } @else if (application(); as item) {
          <h2 class="font-serif text-2xl font-semibold">{{ item.store_name }}</h2>
          <p class="mt-3">Application status: <strong>{{ item.status }}</strong></p>
          @if (item.status === 'pending') {
            <p class="mt-2">Your application is waiting for review.</p>
          }
          @if (item.status === 'approved') {
            <a class="mt-4 inline-block rounded bg-[#a84727] px-5 py-2 text-white"
               routerLink="/seller/dashboard">Manage my shop</a>
          }
          @if (item.decision_reason) {
            <p class="mt-2">Review note: {{ item.decision_reason }}</p>
          }
        } @else {
          <form #form="ngForm" (ngSubmit)="submit()" class="grid gap-4">
            <label class="grid gap-1">
              Store name
              <input name="storeName" [(ngModel)]="storeName" required
                minlength="3" maxlength="120" class="rounded border p-2" />
            </label>
            <label class="grid gap-1">
              Country code (2 letters)
              <input name="country" [(ngModel)]="country" required
                pattern="[A-Za-z]{2}" maxlength="2" placeholder="LK" class="rounded border p-2" />
            </label>
            <label class="grid gap-1">
              Shop description (optional)
              <textarea name="description" [(ngModel)]="description"
                maxlength="2000" rows="4" class="rounded border p-2"></textarea>
            </label>
            <button type="submit" [disabled]="form.invalid || saving()"
              class="w-fit cursor-pointer rounded bg-[#b6502c] px-5 py-2 text-white disabled:opacity-50">
              {{ saving() ? 'Submitting…' : 'Submit application' }}
            </button>
          </form>
        }
      </section>
    </main>
  `,
})
export class SellerApplicationPage implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly destroy = inject(DestroyRef);
  private readonly base = environment.apiBaseUrl.replace(/\/+$/, '') + '/seller';

  readonly state = signal<'loading' | 'ready' | 'error'>('loading');
  readonly application = signal<SellerApplication | null>(null);
  readonly saving = signal(false);
  readonly message = signal('');

  storeName = '';
  country = '';
  description = '';

  ngOnInit(): void {
    this.load();
  }

  load(): void {
    this.state.set('loading');
    this.message.set('');
    this.http.get<ApplicationResponse>(this.base + '/application')
      .pipe(takeUntilDestroyed(this.destroy))
      .subscribe({
        next: ({ application }) => {
          this.application.set(application);
          this.state.set('ready');
        },
        error: (err: HttpErrorResponse) => {
          if (err.status === 404) {
            this.application.set(null);
            this.state.set('ready');
            return;
          }
          this.message.set(err.error?.error || 'Could not load your application.');
          this.state.set('error');
        },
      });
  }

  submit(): void {
    if (this.saving() || this.application() || this.state() !== 'ready') return;

    const store_name = this.storeName.trim();
    const country_code = this.country.trim().toUpperCase();
    const description = this.description.trim();

    if (store_name.length < 3 || store_name.length > 120 || !/^[A-Z]{2}$/.test(country_code)) {
      this.message.set('Enter a store name of 3–120 characters and a two-letter country code.');
      return;
    }

    this.saving.set(true);
    this.message.set('');
    this.http.post<ApplicationResponse>(this.base + '/applications', {
      store_name, country_code, description,
    })
      .pipe(takeUntilDestroyed(this.destroy))
      .subscribe({
        next: ({ application }) => {
          this.application.set(application);
          this.saving.set(false);
        },
        error: (err: HttpErrorResponse) => {
          this.saving.set(false);
          if (err.status === 409) {
            this.load();
            return;
          }
          this.message.set(err.error?.error || 'Could not submit your application.');
        },
      });
  }
}
