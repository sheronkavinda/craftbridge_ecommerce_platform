import { ChangeDetectionStrategy, Component, OnInit, computed, inject, input, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { AccountApiService } from '../../../account/account-api.service';
import { buildInitials } from '../payment-labels';

export type SellerNavItemId =
  | 'dashboard'
  | 'products'
  | 'orders'
  | 'earnings'
  | 'messages'
  | 'reviews'
  | 'shop-settings'
  | 'account-settings';

interface SellerNavItem {
  id: SellerNavItemId;
  label: string;
  route: string | null;
}

// TODO(member-03): Swap for the shared seller workspace shell when Member 03's layout is merged.
@Component({
  selector: 'app-seller-shell',
  standalone: true,
  imports: [RouterLink],
  templateUrl: './seller-shell.component.html',
  styleUrl: './seller-shell.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SellerShellComponent implements OnInit {
  private readonly accountApi = inject(AccountApiService);

  readonly activeItem = input<SellerNavItemId>('earnings');
  readonly mobileMenuOpen = signal<boolean>(false);

  readonly sellerName = signal<string>(this.readStoredUserName() || 'Seller');
  readonly sellerEmail = signal<string>('');
  readonly initials = computed(() => buildInitials(this.sellerName()));

  ngOnInit(): void {
    this.accountApi.me().subscribe({
      next: (user) => {
        const name = user.name?.trim();
        if (name) {
          this.sellerName.set(name);
        }
        const email = user.email?.trim();
        if (email) {
          this.sellerEmail.set(email);
        }
      },
      error: () => {
        // Keep localStorage user_name fallback when profile lookup fails.
      },
    });
  }

  private readStoredUserName(): string {
    if (typeof localStorage === 'undefined') {
      return '';
    }
    return (localStorage.getItem('user_name') ?? '').trim();
  }

  readonly navItems: readonly SellerNavItem[] = [
    { id: 'dashboard', label: 'Dashboard', route: null },
    { id: 'products', label: 'My Products', route: null },
    { id: 'orders', label: 'Orders', route: null },
    { id: 'earnings', label: 'Earnings', route: '/payments/seller-earnings' },
    { id: 'messages', label: 'Messages', route: null },
    { id: 'reviews', label: 'Reviews', route: null },
    { id: 'shop-settings', label: 'Shop Settings', route: null },
    { id: 'account-settings', label: 'Account Settings', route: null },
  ];

  toggleMobileMenu(): void {
    this.mobileMenuOpen.update((open) => !open);
  }
}
