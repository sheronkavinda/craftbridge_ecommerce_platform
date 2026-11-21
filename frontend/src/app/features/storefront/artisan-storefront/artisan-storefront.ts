import { DOCUMENT } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import {
  EMPTY,
  Observable,
  catchError,
  distinctUntilChanged,
  forkJoin,
  map,
  of,
  switchMap,
} from 'rxjs';

import { environment } from '../../../../environments/environment';
import { Header } from '../../../shared/components/header/header';
import {
  ProductCard,
  StorefrontProduct,
} from '../../../shared/components/product-card/product-card';
import {
  PublicCatalogProduct,
  StorefrontApi,
} from '../services/storefront-api';

type StoreTab = 'Products' | 'Our Story' | 'Reviews' | 'Policies';

export interface ArtisanProfile {
  id: string;
  name: string;
  location: string;
  rating: number | null;
  reviewCount: number | null;
  productCount: number | null;
  description: string;
  banner: string | null;
  logo: string | null;
  verified: boolean | null;
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
  private readonly api = inject(StorefrontApi);
  private readonly destroyRef = inject(DestroyRef);
  private readonly document = inject(DOCUMENT);

  readonly tabs: StoreTab[] = ['Products', 'Our Story', 'Reviews', 'Policies'];
  readonly selectedTab = signal<StoreTab>('Products');

  readonly artisanId = signal('');
  readonly artisan = signal<ArtisanProfile | null>(null);
  readonly products = signal<StorefrontProduct[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');

  ngOnInit(): void {
    this.route.paramMap
      .pipe(
        map(params => params.get('id')?.trim() ?? ''),
        distinctUntilChanged(),
        switchMap(slug => {
          this.artisanId.set(slug);
          this.artisan.set(null);
          this.products.set([]);
          this.selectedTab.set('Products');
          this.error.set('');
          this.loading.set(Boolean(slug));

          if (!slug) {
            this.error.set('Shop address is missing.');
            return EMPTY;
          }

          return this.api.getShopBySlug(slug).pipe(
            catchError((err: HttpErrorResponse) => {
              this.error.set(
                err.status === 404
                  ? 'Shop not found.'
                  : 'Could not load this shop. Please try again.'
              );
              this.loading.set(false);
              return EMPTY;
            }),
            switchMap(({ shop }) => {
              this.artisan.set({
                id: shop.slug,
                name: shop.name,
                location: shop.country_code,
                description: shop.description,
                story: shop.description,
                banner: this.mediaUrl(shop.banner_image_url),
                logo: this.mediaUrl(shop.profile_image_url),
                rating: null,
                reviewCount: null,
                productCount: null,
                verified: null,
              });

              return this.api.getShopProducts(shop.id).pipe(
                switchMap(items => {
                  if (items.length === 0) {
                    return of<StorefrontProduct[]>([]);
                  }

                  return forkJoin(items.map(item => this.loadProductCard(item)));
                }),
                catchError(() => {
                  this.error.set(
                    'Could not load the products for this shop. Please try again.'
                  );
                  this.loading.set(false);
                  return EMPTY;
                })
              );
            })
          );
        }),
        takeUntilDestroyed(this.destroyRef)
      )
      .subscribe(cards => {
        this.setProductsData(cards);
        this.loading.set(false);
      });
  }

  selectTab(tab: StoreTab): void {
    this.selectedTab.set(tab);
  }

  setArtisanData(data: ArtisanProfile): void {
    if (data.id === this.artisanId()) {
      this.artisan.set(data);
      this.error.set('');
      this.loading.set(false);
    }
  }

  setProductsData(data: StorefrontProduct[]): void {
    this.products.set(data);
    this.artisan.update(profile =>
      profile ? { ...profile, productCount: data.length } : null
    );
  }

  private loadProductCard(item: PublicCatalogProduct): Observable<StorefrontProduct> {
    return this.api.getProductImages(item.id).pipe(
      map(({ items }) => {
        const [cover] = [...items].sort(
          (a, b) => a.sort_order - b.sort_order || a.id - b.id
        );
        return this.mediaUrl(cover?.image_url ?? null);
      }),
      // Keep the product visible if its image request fails.
      catchError(() => of(null)),
      map((imageUrl): StorefrontProduct => ({
        id: String(item.id),
        name: item.title,
        price: item.price_cents / 100,
        currency: item.currency,
        imageUrl,
        artisanName: item.shop_name,
        rating: null,
        reviewCount: 0,
        isWishlisted: false,
      }))
    );
  }

  private mediaUrl(path: string | null): string | null {
    if (!path) {
      return null;
    }

    const apiUrl = new URL(environment.apiBaseUrl, this.document.baseURI);
    return new URL(path, apiUrl.origin).href;
  }
}