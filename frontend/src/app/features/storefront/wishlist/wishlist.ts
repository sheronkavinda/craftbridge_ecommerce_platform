
import { Component, inject, OnInit, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';

import { Header } from '../../../shared/components/header/header';
import {
  WishlistApi,
  WishlistItem,
} from '../services/wishlist-api';

@Component({
  selector: 'app-wishlist',
  standalone: true,
  imports: [Header, RouterLink],
  templateUrl: './wishlist.html',
  styleUrl: './wishlist.scss',
})
export class Wishlist implements OnInit {
  private readonly wishlistApi = inject(WishlistApi);

  readonly items = signal<WishlistItem[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');
  readonly removingId = signal<string | null>(null);

  ngOnInit(): void {
    this.loadWishlist();
  }

  loadWishlist(): void {
    this.loading.set(true);
    this.error.set('');

    this.wishlistApi.getWishlist().subscribe({
      next: (response) => {
        this.items.set(response.items);
        this.loading.set(false);
      },
      error: (err: HttpErrorResponse) => {
        this.loading.set(false);

        if (err.status === 401) {
          this.error.set(
            'Please sign in to view your wishlist.'
          );
        } else {
          this.error.set(
            'Unable to load your wishlist. Please try again.'
          );
        }
      },
    });
  }

  removeItem(productId: string): void {
    if (this.removingId() !== null) {
      return;
    }

    this.removingId.set(productId);
    this.error.set('');

    this.wishlistApi.removeFromWishlist(productId).subscribe({
      next: () => {
        this.items.update((current) =>
          current.filter((item) => item.productId !== productId)
        );

        this.removingId.set(null);
      },
      error: () => {
        this.error.set(
          'Unable to remove this item. Please try again.'
        );

        this.removingId.set(null);
      },
    });
  }
}
