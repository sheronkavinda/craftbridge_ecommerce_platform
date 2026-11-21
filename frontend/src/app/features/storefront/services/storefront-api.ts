import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { EMPTY, Observable, expand, map, reduce } from 'rxjs';

import { environment } from '../../../../environments/environment';

export interface StorefrontHealthResponse {
  status: string;
  module: string;
  message: string;
}

export interface PublicShop {
  id: number;
  name: string;
  slug: string;
  description: string;
  country_code: string;
  profile_image_url: string | null;
  banner_image_url: string | null;
}

export interface PublicShopResponse {
  shop: PublicShop;
}

export interface PublicCatalogProduct {
  id: number;
  shop_id: number;
  category_id: number;
  title: string;
  description: string;
  price_cents: number;
  currency: string;
  status: string;
  shop_name: string;
  category_name: string;
  available_qty: number;
  reserved_qty: number;
}

export interface PublicProductPage {
  items: PublicCatalogProduct[];
  total: number;
  page: number;
  page_size: number;
}

export interface PublicProductImage {
  id: number;
  product_id: number;
  image_url: string;
  alt_text: string;
  sort_order: number;
}

@Injectable({
  providedIn: 'root',
})
export class StorefrontApi {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = environment.apiBaseUrl.replace(/\/+$/, '');

  getHealth(): Observable<StorefrontHealthResponse> {
    return this.http.get<StorefrontHealthResponse>(
      `${this.baseUrl}/storefront/health`
    );
  }

  getProductById(productId: string): Observable<unknown> {
    return this.http.get<unknown>(
      `${this.baseUrl}/products/${encodeURIComponent(productId)}`
    );
  }

  getShopBySlug(slug: string): Observable<PublicShopResponse> {
    return this.http.get<PublicShopResponse>(
      `${this.baseUrl}/shops/${encodeURIComponent(slug)}`
    );
  }

  getProductImages(productId: number): Observable<{ items: PublicProductImage[] }> {
    return this.http.get<{ items: PublicProductImage[] }>(
      `${this.baseUrl}/products/${productId}/images`
    );
  }

  getShopProducts(shopId: number): Observable<PublicCatalogProduct[]> {
    const getPage = (page: number) =>
      this.http.get<PublicProductPage>(`${this.baseUrl}/products`, {
        params: { page, page_size: 12 },
      });

    return getPage(1).pipe(
      expand((page, index) => {
        const hasMore =
          page.items.length > 0 &&
          page.page_size > 0 &&
          page.page === index + 1 &&
          page.page * page.page_size < page.total;

        return hasMore ? getPage(page.page + 1) : EMPTY;
      }),
      map(page =>
        page.items.filter(
          product => product.shop_id === shopId && product.status === 'published'
        )
      ),
      reduce(
        (products, pageProducts) => products.concat(pageProducts),
        [] as PublicCatalogProduct[]
      )
    );
  }
}