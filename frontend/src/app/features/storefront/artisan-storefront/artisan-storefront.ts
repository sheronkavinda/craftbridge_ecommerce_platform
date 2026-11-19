
import { Component, inject, OnInit, signal } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { Header } from '../../../shared/components/header/header';
import {
  ProductCard,
  StorefrontProduct,
} from '../../../shared/components/product-card/product-card';

type StoreTab = 'Products' | 'Our Story' | 'Reviews' | 'Policies';

export interface ArtisanProfile {
  id: string;
  name: string;
  location: string;
  rating: number | null;
  reviewCount: number;
  productCount: number;
  description: string;
  banner: string | null;
  logo: string | null;
  verified: boolean;
  story: string;
}

@Component({
  selector: 'app-artisan-storefront',
  standalone: true,
  imports: [Header, RouterLink, ProductCard],
  templateUrl: './artisan-storefront.html',
})
export class ArtisanStorefront implements OnInit {
  private readonly route = inject(ActivatedRoute);

  readonly selectedTab = signal<StoreTab>('Products');

  readonly tabs: StoreTab[] = [
    'Products',
    'Our Story',
    'Reviews',
    'Policies',
  ];

  readonly artisanId = signal('');
  readonly artisan = signal<ArtisanProfile | null>(null);
  readonly products = signal<StorefrontProduct[]>([]);

  readonly loading = signal(false);
  readonly error = signal('');

  ngOnInit(): void {
    this.route.paramMap.subscribe((params) => {
      const id = params.get('id')?.trim() ?? '';

      this.artisanId.set(id);
      this.artisan.set(null);
      this.products.set([]);
      this.selectedTab.set('Products');
      this.loading.set(false);

      if (!id) {
        this.error.set('Invalid artisan ID.');
        return;
      }

      // Seller API integration is pending.
      // Do not display fabricated artisan or product data.
      this.error.set(
        'Artisan details are not available yet. Seller API integration is pending.'
      );
    });
  }

  selectTab(tab: StoreTab): void {
    this.selectedTab.set(tab);
  }

  setArtisanData(data: ArtisanProfile): void {
    if (data.id !== this.artisanId()) {
      return;
    }

    this.artisan.set(data);
    this.error.set('');
    this.loading.set(false);
  }

  setProductsData(data: StorefrontProduct[]): void {
    this.products.set(data);
  }
}
