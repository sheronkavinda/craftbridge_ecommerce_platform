
import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../../environments/environment';

export interface StorefrontHealthResponse {
  status: string;
  module: string;
  message: string;
}

@Injectable({
  providedIn: 'root',
})
export class StorefrontApi {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = environment.apiBaseUrl;

  // Existing Storefront Health API
  getHealth(): Observable<StorefrontHealthResponse> {
    return this.http.get<StorefrontHealthResponse>(
      `${this.baseUrl}/storefront/health`
    );
  }

  // Product Detail API - Catalog module
  getProductById(productId: string): Observable<unknown> {
    return this.http.get<unknown>(
      `${this.baseUrl}/products/${encodeURIComponent(productId)}`
    );
  }
}
