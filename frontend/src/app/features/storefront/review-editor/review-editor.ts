
import {
  Component,
  computed,
  inject,
  signal,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';

import { Header } from '../../../shared/components/header/header';

@Component({
  selector: 'app-review-editor',
  standalone: true,
  imports: [FormsModule, RouterLink, Header],
  templateUrl: './review-editor.html',
})
export class ReviewEditor {
  private readonly route = inject(ActivatedRoute);

  readonly productId =
    this.route.snapshot.paramMap.get('productId') ?? '';

  readonly rating = signal(0);
  readonly reviewTitle = signal('');
  readonly reviewText = signal('');

  readonly submitted = signal(false);
  readonly error = signal('');
  readonly loading = signal(false);

  readonly stars = [1, 2, 3, 4, 5];

  readonly ratingLabels = [
    '',
    'Poor',
    'Fair',
    'Good',
    'Very good',
    'Excellent',
  ];

  readonly characterCount = computed(
    () => this.reviewText().length
  );

  readonly canSubmit = computed(() =>
    this.productId.trim().length > 0 &&
    this.rating() >= 1 &&
    this.rating() <= 5 &&
    this.reviewTitle().trim().length >= 3 &&
    this.reviewText().trim().length >= 10 &&
    this.reviewText().trim().length <= 2000 &&
    !this.loading()
  );

  selectRating(value: number): void {
    this.rating.set(value);
    this.error.set('');
  }

  submitReview(): void {
    this.submitted.set(true);
    this.error.set('');

    if (!this.productId.trim()) {
      this.error.set('Invalid product ID.');
      return;
    }

    if (!this.canSubmit()) {
      this.error.set(
        'Please select a rating, enter a title of at least 3 characters, and write a review between 10 and 2000 characters.'
      );
      return;
    }

    // Review backend and verified purchase checking are pending.
    // Do not pretend the review has been saved.
    this.error.set(
      'Review submission is not available yet. Backend integration is pending.'
    );
  }
}
