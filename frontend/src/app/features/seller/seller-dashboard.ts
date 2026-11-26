import { Component, OnInit, inject, signal } from '@angular/core';
import { DecimalPipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { firstValueFrom, forkJoin } from 'rxjs';
import { Header } from '../../shared/components/header/header';
import { SellerApi, errorText } from './seller-api.service';
import { SellerThumbnail } from './seller-thumbnail';
import { Dashboard, Product, Shop } from './seller.models';

@Component({
  standalone: true,
  imports: [Header, RouterLink, FormsModule, DecimalPipe, SellerThumbnail],
  templateUrl: './seller-dashboard.html',
  styleUrl: './seller-dashboard.css',
})
export class SellerDashboard implements OnInit {
  readonly api = inject(SellerApi);
  readonly dash = signal<Dashboard | null>(null);
  readonly products = signal<Product[]>([]);
  readonly busy = signal(false);
  readonly error = signal('');
  readonly message = signal('');
  name = '';
  description = '';
  email = '';
  ngOnInit() {
    void this.load();
  }
  load() {
    return this.run(async () => {
      const { summary, list } = await firstValueFrom(
        forkJoin({ summary: this.api.dashboard(), list: this.api.list() }),
      );
      this.dash.set(summary.dashboard);
      this.products.set(list.items ?? []);
      this.fields(summary.dashboard.shop);
    });
  }
  private fields(s: Shop) {
    this.name = s.name;
    this.description = s.description;
    this.email = s.contact_email ?? '';
  }
  save() {
    return this.run(async () => {
      const { shop } = await firstValueFrom(
        this.api.saveShop(this.name.trim(), this.description, this.email.trim()),
      );
      this.dash.update((d) => d && { ...d, shop });
      this.fields(shop);
      this.message.set('Shop profile saved.');
    });
  }
  upload(kind: 'banner' | 'profile', input: HTMLInputElement) {
    const f = input.files?.[0];
    if (!f) return;
    void this.run(async () => {
      const { shop } = await firstValueFrom(this.api.shopPhoto(kind, f));
      this.dash.update((d) => d && { ...d, shop });
      this.message.set('Shop image saved.');
    });
    input.value = '';
  }
  private async run(action: () => Promise<void>) {
    if (this.busy()) return;
    this.busy.set(true);
    this.error.set('');
    this.message.set('');
    try {
      await action();
    } catch (e) {
      this.error.set(errorText(e));
    } finally {
      this.busy.set(false);
    }
  }
}
