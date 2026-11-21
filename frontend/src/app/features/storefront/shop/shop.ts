
import {
  Component,
  computed,
  inject,
  OnInit,
  signal,
} from '@angular/core';

import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { DOCUMENT } from '@angular/common';
import { DestroyRef } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { EMPTY, catchError, expand, forkJoin, from, map, mergeMap, reduce, switchMap } from 'rxjs';
import { environment } from '../../../../environments/environment';
import { StorefrontApi, type PublicCatalogProduct, type PublicProductPage } from '../services/storefront-api';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';

import { Header } from '../../../shared/components/header/header';

import {
  ProductCard,
  StorefrontProduct,
} from '../../../shared/components/product-card/product-card';

import { WishlistApi } from '../services/wishlist-api';

type SortOption =
  | 'relevant'
  | 'price-low'
  | 'price-high'
  | 'rating'
  | 'name';

interface ShopProduct extends StorefrontProduct {
  category?: string;
  sellerCountry?: string;
  availability?: 'in-stock' | 'made-to-order';
}

@Component({
  selector: 'app-shop',
  standalone: true,
  imports: [FormsModule, Header, ProductCard],
  templateUrl: './shop.html',
  styleUrl: './shop.scss',
})
export class Shop implements OnInit {
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly wishlistApi = inject(WishlistApi);

  private readonly http = inject(HttpClient);
  private readonly catalogApi = inject(StorefrontApi);
  private readonly destroyRef = inject(DestroyRef);
  private readonly baseUrl = environment.apiBaseUrl.replace(/\/+$/, '');
  private readonly mediaOrigin = new URL(this.baseUrl, inject(DOCUMENT).baseURI).origin;

  readonly loading = signal(true);
  readonly loadError = signal('');

  // Wishlist state
  readonly wishlistLoading = signal(false);
  readonly wishlistMessage = signal('');

  ngOnInit(): void {
    this.loadCatalog();
    this.route.queryParamMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
      const search = params.get('search') ?? '';
      this.onSearchChange(search);
    });
  }

  // Real products will be loaded from the Catalog API.
  readonly products = signal<ShopProduct[]>([]);

  // Search and filters
  readonly searchQuery = signal('');
  readonly selectedCategories = signal<string[]>([]);
  readonly minPrice = signal<number | null>(null);
  readonly maxPrice = signal<number | null>(null);
  readonly selectedCountry = signal('');
  readonly selectedAvailability = signal('');
  readonly minimumRating = signal<number | null>(null);

  // Sorting and pagination
  readonly sortBy = signal<SortOption>('relevant');
  readonly currentPage = signal(1);
  readonly pageSize = signal(8);

  readonly categories = signal<{ name: string; value: string }[]>([]);

  loadCatalog(): void {
    this.loading.set(true);
    this.loadError.set('');

    const getPage = (page: number) => this.http.get<PublicProductPage>(
      this.baseUrl + '/products',
      { params: { page, page_size: 50 } }
    );

    const products = getPage(1).pipe(
      expand(page =>
        page.items.length > 0 && page.page * page.page_size < page.total
          ? getPage(page.page + 1)
          : EMPTY
      ),
      reduce((all, page) => all.concat(page.items), [] as PublicCatalogProduct[])
    );

    const categories = this.http.get<{
      items: { id: number; name: string; slug: string }[];
    }>(this.baseUrl + '/categories');

    forkJoin({ products, categories }).pipe(
      switchMap(result => {
        const categorySlugs = new Map(
          result.categories.items.map(category => [category.id, category.slug] as const)
        );

        this.categories.set(result.categories.items.map(category => ({
          name: category.name,
          value: category.slug,
        })));

        this.products.set(result.products.map((item): ShopProduct => ({
          id: String(item.id),
          name: item.title,
          artisanName: item.shop_name,
          price: item.price_cents / 100,
          currency: item.currency,
          category: categorySlugs.get(item.category_id),
          availability: item.available_qty > 0 ? 'in-stock' : undefined,
          imageUrl: null,
          rating: null,
          reviewCount: 0,
          isWishlisted: false,
        })));

        this.loading.set(false);

        return from(result.products).pipe(
          mergeMap(item => this.catalogApi.getProductImages(item.id).pipe(
            map(({ items }) => {
              const [cover] = [...items].sort(
                (a, b) => a.sort_order - b.sort_order || a.id - b.id
              );

              return {
                id: String(item.id),
                url: cover ? new URL(cover.image_url, this.mediaOrigin).href : null,
              };
            }),
            catchError(() => EMPTY)
          ), 4)
        );
      }),
      takeUntilDestroyed(this.destroyRef)
    ).subscribe({
      next: image => {
        if (image.url) {
          this.products.update(products => products.map(product =>
            product.id === image.id ? { ...product, imageUrl: image.url } : product
          ));
        }
      },
      error: () => {
        this.loading.set(false);
        this.loadError.set('Could not load the catalog.');
      },
    });
  }

  readonly sortOptions = [
    { label: 'Most relevant', value: 'relevant' },
    { label: 'Price: Low to High', value: 'price-low' },
    { label: 'Price: High to Low', value: 'price-high' },
    { label: 'Name: A to Z', value: 'name' },
  ];

  // Filter products
  readonly filteredProducts = computed(() => {
    const search = this.searchQuery().trim().toLowerCase();
    const categories = this.selectedCategories();
    const min = this.minPrice();
    const max = this.maxPrice();
    const country = this.selectedCountry();
    const availability = this.selectedAvailability();
    const rating = this.minimumRating();

    const result = this.products().filter((product) => {
      const matchesSearch =
        !search ||
        product.name.toLowerCase().includes(search) ||
        (product.category ?? '').replace(/-/g, ' ').includes(search) ||
        product.artisanName.toLowerCase().includes(search);

      const matchesCategory =
        categories.length === 0 ||
        (
          !!product.category &&
          categories.includes(product.category.toLowerCase())
        );

      const matchesMinPrice =
        min === null || product.price >= min;

      const matchesMaxPrice =
        max === null || product.price <= max;

      const matchesCountry =
        !country ||
        product.sellerCountry?.toLowerCase() ===
          country.toLowerCase();

      const matchesAvailability =
        !availability ||
        product.availability === availability;

      const matchesRating =
        rating === null ||
        (
          product.rating !== null &&
          product.rating >= rating
        );

      return (
        matchesSearch &&
        matchesCategory &&
        matchesMinPrice &&
        matchesMaxPrice &&
        matchesCountry &&
        matchesAvailability &&
        matchesRating
      );
    });

    // Sort products
    switch (this.sortBy()) {
      case 'price-low':
        return [...result].sort(
          (a, b) => a.price - b.price
        );

      case 'price-high':
        return [...result].sort(
          (a, b) => b.price - a.price
        );

      case 'rating':
        return [...result].sort(
          (a, b) => (b.rating ?? 0) - (a.rating ?? 0)
        );

      case 'name':
        return [...result].sort(
          (a, b) => a.name.localeCompare(b.name)
        );

      default:
        return result;
    }
  });

  // Results count
  readonly totalResults = computed(
    () => this.filteredProducts().length
  );

  // Total pages
  readonly totalPages = computed(() =>
    Math.max(
      1,
      Math.ceil(
        this.totalResults() / this.pageSize()
      )
    )
  );

  // Paginated products
  readonly paginatedProducts = computed(() => {
    const start =
      (this.currentPage() - 1) * this.pageSize();

    return this.filteredProducts().slice(
      start,
      start + this.pageSize()
    );
  });

  // Page numbers
  readonly pages = computed(() =>
    Array.from(
      { length: this.totalPages() },
      (_, index) => index + 1
    )
  );

  // Active filters
  readonly activeFilters = computed(() => {
    const filters: string[] = [];

    if (this.searchQuery().trim()) {
      filters.push(
        `Search: ${this.searchQuery().trim()}`
      );
    }

    filters.push(...this.selectedCategories());

    if (this.minPrice() !== null) {
      filters.push(`Min: ${this.minPrice()}`);
    }

    if (this.maxPrice() !== null) {
      filters.push(`Max: ${this.maxPrice()}`);
    }

    if (this.selectedCountry()) {
      filters.push(this.selectedCountry());
    }

    if (this.selectedAvailability()) {
      filters.push(this.selectedAvailability());
    }

    if (this.minimumRating() !== null) {
      filters.push(
        `${this.minimumRating()}+ stars`
      );
    }

    return filters;
  });

  // Search
  onSearchChange(value: string): void {
    this.searchQuery.set(value);
    this.currentPage.set(1);
  }

  // Category filter
  toggleCategory(category: string): void {
    const current = this.selectedCategories();

    this.selectedCategories.set(
      current.includes(category)
        ? current.filter(
            (item) => item !== category
          )
        : [...current, category]
    );

    this.currentPage.set(1);
  }

  // Minimum price
  onMinPriceChange(value: string): void {
    this.minPrice.set(
      value.trim() === ''
        ? null
        : Number(value)
    );

    this.currentPage.set(1);
  }

  // Maximum price
  onMaxPriceChange(value: string): void {
    this.maxPrice.set(
      value.trim() === ''
        ? null
        : Number(value)
    );

    this.currentPage.set(1);
  }

  // Country filter
  onCountryChange(value: string): void {
    this.selectedCountry.set(value);
    this.currentPage.set(1);
  }

  // Availability filter
  onAvailabilityChange(value: string): void {
    this.selectedAvailability.set(value);
    this.currentPage.set(1);
  }

  // Rating filter
  onRatingChange(value: number | null): void {
    this.minimumRating.set(value);
    this.currentPage.set(1);
  }

  // Sort products
  onSortChange(value: string): void {
    this.sortBy.set(value as SortOption);
    this.currentPage.set(1);
  }

  // Page size
  onPageSizeChange(value: string): void {
    this.pageSize.set(Number(value));
    this.currentPage.set(1);
  }

  // Pagination
  goToPage(page: number): void {
    if (
      page >= 1 &&
      page <= this.totalPages()
    ) {
      this.currentPage.set(page);
    }
  }

  // Remove individual filter
  removeFilter(filter: string): void {
    if (filter.startsWith('Search: ')) {
      this.searchQuery.set('');

    } else if (
      this.selectedCategories().includes(filter)
    ) {
      this.selectedCategories.update(
        (items) =>
          items.filter(
            (item) => item !== filter
          )
      );

    } else if (filter.startsWith('Min: ')) {
      this.minPrice.set(null);

    } else if (filter.startsWith('Max: ')) {
      this.maxPrice.set(null);

    } else if (
      filter === this.selectedCountry()
    ) {
      this.selectedCountry.set('');

    } else if (
      filter === this.selectedAvailability()
    ) {
      this.selectedAvailability.set('');

    } else if (
      filter.endsWith('+ stars')
    ) {
      this.minimumRating.set(null);
    }

    this.currentPage.set(1);
  }

  // Clear all filters
  clearFilters(): void {
    this.searchQuery.set('');
    this.selectedCategories.set([]);
    this.minPrice.set(null);
    this.maxPrice.set(null);
    this.selectedCountry.set('');
    this.selectedAvailability.set('');
    this.minimumRating.set(null);
    this.currentPage.set(1);

    if (this.route.snapshot.queryParamMap.has('search')) {
      void this.router.navigate([], {
        relativeTo: this.route,
        queryParams: { search: null },
        queryParamsHandling: 'merge',
        replaceUrl: true,
      });
    }
  }

  // Wishlist Add / Remove
  onWishlistClick(productId: string): void {
    if (this.wishlistLoading()) {
      return;
    }

    const product = this.products().find(
      (item) => item.id === productId
    );

    if (!product) {
      return;
    }

    this.wishlistLoading.set(true);
    this.wishlistMessage.set('');

    // Remove from wishlist
    if (product.isWishlisted) {
      this.wishlistApi
        .removeFromWishlist(productId)
        .subscribe({
          next: () => {
            this.products.update((items) =>
              items.map((item) =>
                item.id === productId
                  ? {
                      ...item,
                      isWishlisted: false,
                    }
                  : item
              )
            );

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
      .subscribe({
        next: () => {
          this.products.update((items) =>
            items.map((item) =>
              item.id === productId
                ? {
                    ...item,
                    isWishlisted: true,
                  }
                : item
            )
          );

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

  // Wishlist error handling
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
}
