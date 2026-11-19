
import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../../environments/environment';

export interface WishlistItem {
  id: number;
  userId: string;
  productId: string;
  createdAt: string;
}

export interface WishlistResponse {
  items: WishlistItem[];
  total: number;
}

@Injectable({
  providedIn: 'root',
})
export class WishlistApi {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = environment.apiBaseUrl;

  getWishlist(): Observable<WishlistResponse> {
    return this.http.get<WishlistResponse>(
      `${this.baseUrl}/me/wishlist`
    );
  }

  addToWishlist(productId: string): Observable<WishlistItem> {
    return this.http.post<WishlistItem>(
      `${this.baseUrl}/me/wishlist`,
      { productId }
    );
  }

  removeFromWishlist(productId: string): Observable<void> {
    return this.http.delete<void>(
      `${this.baseUrl}/me/wishlist/${encodeURIComponent(productId)}`
    );
  }
}
