import { Component, OnInit, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { Header } from '../../shared/components/header/header';
import { SellerApi, errorText } from './seller-api.service';

@Component({
  standalone: true,
  imports: [Header, RouterLink],
  styleUrl: './seller-workspace.css',
  template: ` <app-header />
    <main>
      <a routerLink="/seller/dashboard">Back to seller dashboard</a>
      <h1>Fulfillment orders</h1>
      @if (loading()) {
        <p role="status">Loading orders…</p>
      } @else if (error()) {
        <p role="alert">{{ error() }}</p>
        <button (click)="load()">Retry</button>
      } @else {
        <section>
          <h2>Paid orders</h2>
          @for (o of orders(); track o.id) {
            <article>
              Order #{{ o.order_id }} <span>{{ o.status }}</span>
            </article>
          } @empty {
            <p>No paid orders to fulfill.</p>
          }
        </section>
      }
    </main>`,
})
export class SellerOrders implements OnInit {
  private api = inject(SellerApi);
  readonly orders = signal<{ id: number; order_id: number; status: string }[]>([]);
  readonly loading = signal(true);
  readonly error = signal('');
  ngOnInit() {
    this.load();
  }
  load() {
    this.loading.set(true);
    this.error.set('');
    this.api.orders().subscribe({
      next: (r) => {
        this.orders.set(r.items ?? []);
        this.loading.set(false);
      },
      error: (e) => {
        this.error.set(errorText(e));
        this.loading.set(false);
      },
    });
  }
}
