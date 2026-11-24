import { CurrencyPipe } from '@angular/common';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Component, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { RouterLink } from '@angular/router';
import { environment } from '../../../environments/environment';
import { Header } from '../../shared/components/header/header';

interface CartItem {
  id: number;
  product_id: number;
  title: string;
  shop_name: string;
  quantity: number;
  unit_price_cents: number;
  line_total_cents: number;
  is_available: boolean;
}

interface CartSummary {
  items: CartItem[] | null;
  subtotal_cents: number;
  currency: string;
}

@Component({
  selector: 'app-cart-page',
  standalone: true,
  imports: [Header, RouterLink, CurrencyPipe],
  template: `
    <app-header />
    <main class="min-h-screen bg-[#faf8f5] px-5 py-10 text-[#14213a]">
      <div class="mx-auto max-w-3xl">
        <a routerLink="/shop" class="text-sm underline">Continue shopping</a>
        <h1 class="my-6 font-serif text-4xl font-bold">Your cart</h1>

        @if (error()) {
          <p role="alert">{{ error() }}</p>
          @if (signInRequired()) {
            <a routerLink="/login" class="mt-3 inline-block underline">Sign in</a>
          }
        } @else if (cart(); as summary) {
          @for (item of summary.items ?? []; track item.id) {
            <article class="mb-4 rounded-xl border border-stone-200 bg-white p-5">
              <p class="text-sm">{{ item.shop_name }}</p>
              <h2 class="mt-1 text-xl font-semibold">
                <a [routerLink]="['/products', item.product_id]" class="hover:underline">
                  {{ item.title }}
                </a>
              </h2>
              <p class="mt-3">
                Quantity {{ item.quantity }} ·
                {{ item.unit_price_cents / 100 | currency:summary.currency:'code' }} each
              </p>
              <p class="mt-1 font-semibold">
                Line total {{ item.line_total_cents / 100 | currency:summary.currency:'code' }}
              </p>
              @if (!item.is_available) {
                <p class="mt-2">Requested quantity unavailable</p>
              }
            </article>
          } @empty {
            <p>Your cart is empty.</p>
          }

          @if (summary.items?.length) {
            <p class="mt-6 text-right text-xl font-bold">
              Subtotal {{ summary.subtotal_cents / 100 | currency:summary.currency:'code' }}
            </p>
          }
        } @else {
          <p role="status">Loading your cart…</p>
        }
      </div>
    </main>
  `,
})
export class CartPage {
  readonly cart = signal<CartSummary | null>(null);
  readonly error = signal('');
  readonly signInRequired = signal(false);

  constructor() {
    const base = environment.apiBaseUrl.replace(/\/+$/, '');
    inject(HttpClient).get<CartSummary>(base + '/me/cart')
      .pipe(takeUntilDestroyed())
      .subscribe({
        next: summary => this.cart.set(summary),
        error: (err: HttpErrorResponse) => {
          this.signInRequired.set(err.status === 401);
          this.error.set(err.status === 401
            ? 'Sign in to view your cart.'
            : 'Could not load your cart. Refresh to try again.');
        },
      });
  }
}
