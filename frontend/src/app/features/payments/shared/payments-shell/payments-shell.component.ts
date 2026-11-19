import { ChangeDetectionStrategy, Component } from '@angular/core';

// TODO(member-03): Swap for the shared public storefront shell when Member 03's layout is merged.
@Component({
  selector: 'app-payments-shell',
  standalone: true,
  templateUrl: './payments-shell.component.html',
  styleUrl: './payments-shell.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PaymentsShellComponent {
  readonly primaryCategories = [
    { label: 'Handmade Crafts', hasDropdown: true },
    { label: 'Home & Living', hasDropdown: true },
    { label: 'Fashion & Accessories', hasDropdown: true },
    { label: 'Art & Wall Decor', hasDropdown: true },
    { label: 'Collections', hasDropdown: true },
    { label: 'Meet the Artisans', hasDropdown: false },
  ];
}
