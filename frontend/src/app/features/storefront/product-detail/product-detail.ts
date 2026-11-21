import { DOCUMENT } from '@angular/common';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import {
  Component,
  computed,
  DestroyRef,
  inject,
  OnInit,
  signal,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import {
  catchError,
  distinctUntilChanged,
  EMPTY,
  map,
  of,
  switchMap,
  type Observable,
} from 'rxjs';

import { environment } from '../../../../environments/environment';
import { Header } from '../../../shared/components/header/header';
import { StorefrontApi } from '../services/storefront-api';
import { WishlistApi } from '../services/wishlist-api';

interface ProductDetailData {
  id: string;
  name: string;
  price: number;
  currency: string;
  sellerName: string;
  rating: number | null;
  reviewCount: number;
  stockQuantity: number | null;
  images: string[];
  description: string;
  materials: string;
  origin: string;
  handmadeStory: string;
  dimensions: string;
  returnsPolicy: string;
  sizes: string[];
}

function readCatalogProduct(response: unknown) {
  if (!response || typeof response !== 'object' || !('product' in response)) {
    throw new Error('Missing product in catalog response.');
  }

  const item = response.product;

  if (
    !item ||
    typeof item !== 'object' ||
    !('id' in item) || typeof item.id !== 'number' ||
    !('title' in item) || typeof item.title !== 'string' ||
    !('price_cents' in item) || typeof item.price_cents !== 'number' ||
    !('currency' in item) || typeof item.currency !== 'string' ||
    !('shop_name' in item) || typeof item.shop_name !== 'string' ||
    !('available_qty' in item) || typeof item.available_qty !== 'number'
  ) {
    throw new Error('Invalid product in catalog response.');
  }

  return {
    id: item.id,
    name: item.title,
    price: item.price_cents / 100,
    currency: item.currency,
    sellerName: item.shop_name,
    stockQuantity: item.available_qty,
    description:
      'description' in item && typeof item.description === 'string'
        ? item.description
        : '',
  };
}

@Component({
  selector: 'app-product-detail',
  standalone: true,
  imports: [Header, RouterLink],
  templateUrl: './product-detail.html',
  styleUrl: './product-detail.scss',
})
export class ProductDetail implements OnInit {
  private readonly route = inject(ActivatedRoute);
  private readonly storefrontApi = inject(StorefrontApi);
  private readonly wishlistApi = inject(WishlistApi);
  private readonly destroyRef = inject(DestroyRef);
  private readonly document = inject(DOCUMENT);

  get productId(): string | null {
    return this.route.snapshot.paramMap.get('id');
  }

  readonly product = signal<ProductDetailData | null>(null);
  readonly loading = signal(true);
  readonly error = signal('');

  private readonly cartHttp = inject(HttpClient);
  readonly cartPending = signal(false);
  readonly cartMessage = signal('');

  readonly selectedImageIndex = signal(0);
  readonly selectedSize = signal('');
  readonly quantity = signal(1);

  readonly isWishlisted = signal(false);
  readonly wishlistLoading = signal(false);
  readonly wishlistMessage = signal('');

  readonly selectedImage = computed(() => {
    const images = this.product()?.images ?? [];
    return images[this.selectedImageIndex()] ?? null;
  });

  ngOnInit(): void {
    this.route.paramMap
      .pipe(
        map(params => params.get('id')?.trim() ?? ''),
        distinctUntilChanged(),
        switchMap(id => {
          this.product.set(null);
          this.loading.set(true);
          this.error.set('');
          this.cartPending.set(false);
          this.cartMessage.set('');
          this.selectedImageIndex.set(0);
          this.selectedSize.set('');
          this.quantity.set(1);
          this.isWishlisted.set(false);
          this.wishlistLoading.set(false);
          this.wishlistMessage.set('');

          if (!id) {
            this.error.set('Invalid product ID.');
            this.loading.set(false);
            return EMPTY;
          }

          return this.storefrontApi.getProductById(id).pipe(
            map(readCatalogProduct),
            switchMap(item =>
              this.storefrontApi.getProductImages(item.id).pipe(
                map(({ items }) => {
                  const apiOrigin = new URL(
                    environment.apiBaseUrl,
                    this.document.baseURI
                  ).origin;

                  return [...items]
                    .sort((a, b) => a.sort_order - b.sort_order || a.id - b.id)
                    .map(image => new URL(image.image_url, apiOrigin).href);
                }),
                // Keep the product visible if the gallery request fails.
                catchError(() => of<string[]>([])),
                map((images): ProductDetailData => ({
                  id: String(item.id),
                  name: item.name,
                  price: item.price,
                  currency: item.currency,
                  sellerName: item.sellerName,
                  stockQuantity: item.stockQuantity,
                  description: item.description,
                  images,
                  rating: null,
                  reviewCount: 0,
                  materials: '',
                  origin: '',
                  handmadeStory: '',
                  dimensions: '',
                  returnsPolicy: '',
                  sizes: [],
                }))
              )
            ),
            catchError((err: unknown) => {
              const status = err instanceof HttpErrorResponse ? err.status : undefined;

              if (status === 404) {
                this.error.set('Product not found.');
              } else if (status === 0) {
                this.error.set('Cannot connect to the backend. Please check the server.');
              } else {
                this.error.set('Unable to load product details. Please try again later.');
              }

              this.loading.set(false);
              return EMPTY;
            })
          );
        }),
        takeUntilDestroyed(this.destroyRef)
      )
      .subscribe(product => {
        this.product.set(product);
        this.loading.set(false);
      });
  }

  selectImage(index: number): void {
    this.selectedImageIndex.set(index);
  }

  selectSize(size: string): void {
    this.selectedSize.set(size);
  }

  increaseQuantity(): void {
    const item = this.product();

    if (item && (item.stockQuantity === null || this.quantity() < item.stockQuantity)) {
      this.quantity.update(value => value + 1);
    }
  }

  decreaseQuantity(): void {
    this.quantity.update(value => Math.max(1, value - 1));
  }

  toggleWishlist(): void {
    const item = this.product();

    if (!item || this.wishlistLoading()) {
      return;
    }

    const removing = this.isWishlisted();
    this.wishlistLoading.set(true);
    this.wishlistMessage.set('');

    const request: Observable<unknown> = removing
      ? this.wishlistApi.removeFromWishlist(item.id)
      : this.wishlistApi.addToWishlist(item.id);

    request
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: () => {
          if (this.product() !== item) {
            return;
          }

          this.isWishlisted.set(!removing);
          this.wishlistMessage.set(
            removing ? 'Removed from wishlist.' : 'Added to wishlist.'
          );
          this.wishlistLoading.set(false);
        },
        error: (err: HttpErrorResponse) => {
          if (this.product() !== item) {
            return;
          }

          this.wishlistMessage.set(
            err.status === 401
              ? 'Please sign in to manage your wishlist.'
              : 'Unable to update wishlist. Please try again.'
          );
          this.wishlistLoading.set(false);
        },
      });
  }

  addToCart(): void {
    const item = this.product();
    if (!item || this.cartPending()) {
      return;
    }

    const quantity = this.quantity();
    const maximum = Math.min(item.stockQuantity ?? 99, 99);
    if (maximum < 1) {
      this.cartMessage.set('This product is out of stock.');
      return;
    }
    if (!Number.isInteger(quantity) || quantity < 1 || quantity > maximum) {
      this.cartMessage.set(`Choose a quantity between 1 and ${maximum}.`);
      return;
    }

    this.cartPending.set(true);
    this.cartMessage.set('');

    const baseUrl = environment.apiBaseUrl.replace(/\/+$/, '');
    this.cartHttp.post(`${baseUrl}/me/cart/items`, {
      product_id: Number(item.id),
      quantity,
    })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: () => {
          if (this.product() !== item) return;
          this.cartPending.set(false);
          this.cartMessage.set(`Added to cart: ${item.name} (quantity ${quantity}).`);
        },
        error: (err: HttpErrorResponse) => {
          if (this.product() !== item) return;
          this.cartPending.set(false);

          if (err.status === 401) {
            this.cartMessage.set('Please sign in to add items to your cart.');
          } else if (err.status === 409) {
            this.cartMessage.set('This quantity is unavailable. Check your cart and available stock.');
          } else if (err.status === 0) {
            this.cartMessage.set('Connection lost. Check your cart before trying again.');
          } else {
            this.cartMessage.set('Could not add this item. Please check your cart and try again.');
          }
        },
      });
  }
  buyNow(): void {
    // TODO: Connect the checkout API.
    console.log('Buy now:', this.productId);
  }
}