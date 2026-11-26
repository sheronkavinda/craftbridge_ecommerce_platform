import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { environment } from '../../../environments/environment';
import {
  Category,
  Dashboard,
  Movement,
  Photo,
  Product,
  ProductDetails,
  Shop,
} from './seller.models';

@Injectable({ providedIn: 'root' })
export class SellerApi {
  private http = inject(HttpClient);
  private base = environment.apiBaseUrl.replace(/\/+$/, '');
  private seller = this.base + '/seller';
  private products = this.seller + '/products';
  dashboard() {
    return this.http.get<{ dashboard: Dashboard }>(this.seller + '/dashboard');
  }
  shop() {
    return this.http.get<{ shop: Shop }>(this.seller + '/shop');
  }
  saveShop(name: string, description: string, contact_email: string) {
    return this.http.patch<{ shop: Shop }>(this.seller + '/shop', {
      name,
      description,
      contact_email,
    });
  }
  private form(file: File) {
    const data = new FormData();
    data.append('image', file);
    return data;
  }
  shopPhoto(kind: 'banner' | 'profile', file: File) {
    return this.http.post<{ shop: Shop }>(this.seller + '/shop/images/' + kind, this.form(file));
  }
  orders() {
    return this.http.get<{ items: { id: number; order_id: number; status: string }[] }>(
      this.seller + '/orders',
    );
  }
  categories() {
    return this.http.get<{ items: Category[] }>(this.base + '/categories');
  }
  list() {
    return this.http.get<{ items: Product[] }>(this.products);
  }
  detail(id: number) {
    return this.http.get<{ product: Product }>(this.products + '/' + id);
  }
  create(details: ProductDetails, stock: number) {
    return this.http.post<{ product: Product }>(this.products, {
      ...details,
      stock_quantity: stock,
    });
  }
  save(id: number, details: ProductDetails, status: Product['status']) {
    return this.http.patch(this.products + '/' + id, { ...details, status });
  }
  archive(id: number) {
    return this.http.delete(this.products + '/' + id);
  }
  photos(id: number) {
    return this.http.get<{ items: Photo[] }>(this.products + '/' + id + '/images');
  }
  upload(id: number, file: File, alt: string) {
    return this.http.post(this.products + '/' + id + '/images', this.form(file), {
      params: { alt_text: alt },
    });
  }
  removePhoto(id: number) {
    return this.http.delete(this.seller + '/product-images/' + id);
  }
  ledger(id: number) {
    return this.http.get<{ items: Movement[] }>(this.products + '/' + id + '/inventory-movements');
  }
  adjust(id: number, delta: number, reason: string) {
    return this.http.post(this.products + '/' + id + '/inventory-adjustments', { delta, reason });
  }
  media(path: string) {
    return new URL(path, new URL(this.base, document.baseURI)).href;
  }
}
export function errorText(e: unknown): string {
  if (e instanceof HttpErrorResponse)
    return (
      e.error?.error || e.error?.message || 'Could not reach the seller service. Please try again.'
    );
  return e instanceof Error ? e.message : 'Request failed.';
}
