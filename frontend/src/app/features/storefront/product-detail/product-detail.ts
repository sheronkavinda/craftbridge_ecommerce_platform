
import {
  Component,
  computed,
  DestroyRef,
  inject,
  OnInit,
  signal,
} from '@angular/core';

import { HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';

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

  readonly productId = this.route.snapshot.paramMap.get('id');

  // Real product data only
  readonly product = signal<ProductDetailData | null>(null);

  // Product loading state
  readonly loading = signal(true);
  readonly error = signal('');

  // Product selection
  readonly selectedImageIndex = signal(0);
  readonly selectedSize = signal('');
  readonly quantity = signal(1);

  // Wishlist state
  readonly isWishlisted = signal(false);
  readonly wishlistLoading = signal(false);
  readonly wishlistMessage = signal('');

  // Selected product image
  readonly selectedImage = computed(() => {
    const images = this.product()?.images ?? [];

    return images[this.selectedImageIndex()] ?? null;
  });

  ngOnInit(): void {
    this.loadProduct();
  }

  // Load product from Catalog API
  private loadProduct(): void {
    if (!this.productId) {
      this.loading.set(false);
      this.error.set('Invalid product ID.');
      return;
    }

    this.loading.set(true);
    this.error.set('');
    this.product.set(null);

    this.storefrontApi
      .getProductById(this.productId)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (response: unknown) => {
          // Catalog API response structure is not confirmed yet.
          // Do not map unknown data to ProductDetailData.

          console.log('Catalog API response:', response);

          this.loading.set(false);

          this.error.set(
            'Catalog API responded. Product data mapping is pending.'
          );
        },

        error: (err: HttpErrorResponse) => {
          this.loading.set(false);

          if (err.status === 404) {
            this.error.set('Product not found.');
          } else if (err.status === 0) {
            this.error.set(
              'Cannot connect to the backend. Please check the server.'
            );
          } else {
            this.error.set(
              'Unable to load product details. Please try again later.'
            );
          }

          console.error('Product API error:', err);
        },
      });
  }

  // Select gallery image
  selectImage(index: number): void {
    this.selectedImageIndex.set(index);
  }

  // Select product size
  selectSize(size: string): void {
    this.selectedSize.set(size);
  }

  // Increase quantity
  increaseQuantity(): void {
    const stock = this.product()?.stockQuantity;

    if (
      stock === null ||
      stock === undefined ||
      this.quantity() < stock
    ) {
      this.quantity.update((value) => value + 1);
    }
  }

  // Decrease quantity
  decreaseQuantity(): void {
    this.quantity.update((value) =>
      Math.max(1, value - 1)
    );
  }

  // Add or remove product from wishlist
  toggleWishlist(): void {
    const productId = this.productId;

    if (
      !productId ||
      !this.product() ||
      this.wishlistLoading()
    ) {
      return;
    }

    this.wishlistLoading.set(true);
    this.wishlistMessage.set('');

    // Remove from wishlist
    if (this.isWishlisted()) {
      this.wishlistApi
        .removeFromWishlist(productId)
        .pipe(takeUntilDestroyed(this.destroyRef))
        .subscribe({
          next: () => {
            this.isWishlisted.set(false);

            this.wishlistMessage.set(
              'Removed from wishlist.'
            );

            this.wishlistLoading.set(false);
          },

          error: (err: HttpErrorResponse) => {
            this.handleWishlistError(err);
          },
        });

      return;
    }

    // Add to wishlist
    this.wishlistApi
      .addToWishlist(productId)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: () => {
          this.isWishlisted.set(true);

          this.wishlistMessage.set(
            'Added to wishlist.'
          );

          this.wishlistLoading.set(false);
        },

        error: (err: HttpErrorResponse) => {
          this.handleWishlistError(err);
        },
      });
  }

  // Handle wishlist API errors
  private handleWishlistError(
    err: HttpErrorResponse
  ): void {
    this.wishlistLoading.set(false);

    if (err.status === 401) {
      this.wishlistMessage.set(
        'Please sign in to manage your wishlist.'
      );
    } else {
      this.wishlistMessage.set(
        'Unable to update wishlist. Please try again.'
      );
    }
  }

  // Cart integration belongs to Member 04
  addToCart(): void {
    // TODO: Connect Member 04 cart API.

    console.log('Add to cart:', {
      productId: this.productId,
      quantity: this.quantity(),
      size: this.selectedSize(),
    });
  }

  // Checkout integration pending
  buyNow(): void {
    // TODO: Connect checkout API.

    console.log('Buy now:', this.productId);
  }
}
