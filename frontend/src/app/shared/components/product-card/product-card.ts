
import { Component, input, output } from '@angular/core';
import { RouterLink } from '@angular/router';

export interface StorefrontProduct {
  id: string;
  name: string;
  price: number;
  currency: string;
  imageUrl: string | null;
  artisanName: string;
  rating: number | null;
  reviewCount: number;
  isWishlisted: boolean;
}

@Component({
  selector: 'app-product-card',
  standalone: true,
  imports: [RouterLink],
  templateUrl: './product-card.html',
  styleUrl: './product-card.scss',
})
export class ProductCard {
  readonly product = input.required<StorefrontProduct>();

  readonly wishlistClicked = output<string>();

  onWishlistClick(): void {
    this.wishlistClicked.emit(this.product().id);
  }
}
