import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { firstValueFrom, forkJoin } from 'rxjs';
import { Header } from '../../shared/components/header/header';
import { SellerApi, errorText } from './seller-api.service';
import { Category, Movement, Photo, Product, toCents } from './seller.models';
@Component({
  standalone: true,
  imports: [Header, FormsModule, DatePipe, RouterLink],
  templateUrl: './product-editor.html',
  styleUrl: './seller-workspace.css',
})
export class ProductEditor implements OnInit {
  readonly api = inject(SellerApi);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  readonly id = Number(this.route.snapshot.paramMap.get('id'));
  readonly busy = signal(false);
  readonly ready = signal(false);
  readonly error = signal('');
  readonly message = signal('');
  readonly item = signal<Product | null>(null);
  readonly categories = signal<Category[]>([]);
  readonly images = signal<Photo[]>([]);
  readonly history = signal<Movement[]>([]);
  title = '';
  description = '';
  category: number | null = null;
  price = '';
  stock: number | null = 0;
  status: Product['status'] = 'draft';
  delta: number | null = null;
  reason = '';
  alt = '';
  ngOnInit() {
    void this.load();
  }
  load() {
    return this.run(async () => {
      if (this.route.snapshot.paramMap.has('id') && (!Number.isSafeInteger(this.id) || this.id < 1))
        throw Error('Invalid product ID.');
      this.categories.set((await firstValueFrom(this.api.categories())).items ?? []);
      if (this.id) await this.refresh(true);
      this.ready.set(true);
    });
  }
  private async refresh(resetForm: boolean) {
    const { detail, images, ledger } = await firstValueFrom(
      forkJoin({
        detail: this.api.detail(this.id),
        images: this.api.photos(this.id),
        ledger: this.api.ledger(this.id),
      }),
    );
    const p = detail.product;
    this.item.set(p);
    this.images.set(images.items ?? []);
    this.history.set(ledger.items ?? []);
    if (resetForm) {
      this.title = p.title;
      this.description = p.description;
      this.category = p.category_id;
      this.price = (p.price_cents / 100).toFixed(2);
      this.status = p.status;
    }
  }
  save() {
    return this.run(async () => {
      const cents = toCents(this.price);
      if (!this.category || cents === null)
        throw Error('Choose a category and enter a positive price with up to two decimal places.');
      const details = {
        category_id: this.category,
        title: this.title.trim(),
        description: this.description.trim(),
        price_cents: cents,
      };
      if (!this.id) {
        if (this.stock === null || !Number.isInteger(this.stock) || this.stock < 0)
          throw Error('Enter a whole opening quantity.');
        const { product } = await firstValueFrom(this.api.create(details, this.stock));
        await this.router.navigate(['/seller/products', product.id], { replaceUrl: true });
        return;
      }
      await firstValueFrom(this.api.save(this.id, details, this.status));
      this.message.set('Product saved.');
      await this.refresh(true);
    });
  }
  archive() {
    if (!window.confirm('Archive this product and remove it from the public catalog?')) return;
    void this.run(async () => {
      await firstValueFrom(this.api.archive(this.id));
      await this.router.navigate(['/seller/dashboard']);
    });
  }
  adjust() {
    return this.run(async () => {
      if (!this.delta || !Number.isInteger(this.delta) || !this.reason.trim())
        throw Error('Enter a non-zero whole quantity and a reason.');
      await firstValueFrom(this.api.adjust(this.id, this.delta, this.reason.trim()));
      this.delta = null;
      this.reason = '';
      this.message.set('Stock adjustment recorded.');
      await this.refresh(false);
    });
  }
  upload(input: HTMLInputElement) {
    const f = input.files?.[0];
    if (!f) return;
    void this.run(async () => {
      await firstValueFrom(this.api.upload(this.id, f, this.alt.trim()));
      input.value = '';
      this.alt = '';
      this.message.set('Image uploaded.');
      await this.refresh(false);
    });
  }
  remove(image: Photo) {
    if (!window.confirm('Remove this product image?')) return;
    void this.run(async () => {
      await firstValueFrom(this.api.removePhoto(image.id));
      this.images.update((items) => items.filter((i) => i.id !== image.id));
      this.message.set('Image removed.');
    });
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
