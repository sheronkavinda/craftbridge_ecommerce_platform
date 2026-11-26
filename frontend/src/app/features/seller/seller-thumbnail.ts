import { AsyncPipe } from '@angular/common';
import { Component, computed, inject, input } from '@angular/core';
import { catchError, map, of } from 'rxjs';
import { SellerApi } from './seller-api.service';

@Component({
  selector: 'seller-thumbnail',
  standalone: true,
  imports: [AsyncPipe],
  template: `@if (source() | async; as url) { <img [src]="url" alt="" /> }`,
  styles: `:host { display:block; width:100%; aspect-ratio:1; border-radius:.65rem; overflow:hidden; background:#efe7db; }
    img { width:100%; height:100%; object-fit:cover; }`,
})
export class SellerThumbnail {
  readonly productId = input.required<number>();
  private readonly api = inject(SellerApi);
  readonly source = computed(() => this.api.photos(this.productId()).pipe(
    map(({ items }) => items[0] ? this.api.media(items[0].image_url) : ''),
    catchError(() => of('')),
  ));
}
