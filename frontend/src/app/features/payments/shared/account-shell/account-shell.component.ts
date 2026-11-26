import { NgTemplateOutlet } from '@angular/common';
import { ChangeDetectionStrategy, Component, OnInit, computed, inject, input, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { AccountApiService } from '../../../account/account-api.service';
import { buildInitials } from '../payment-labels';

export type AccountNavItemId =
  | 'orders'
  | 'payments'
  | 'refunds'
  | 'returns'
  | 'addresses'
  | 'notifications'
  | 'settings';

interface AccountNavItem {
  id: AccountNavItemId;
  label: string;
  /** App route; null means a visual-only placeholder link (href="#"). */
  route: string | null;
  icon: 'box' | 'card' | 'refresh' | 'pin' | 'bell' | 'user';
}

// TODO(member-01/member-03): Swap for the shared account shell and real signed-in user once merged.
@Component({
  selector: 'app-account-shell',
  standalone: true,
  imports: [RouterLink, NgTemplateOutlet],
  templateUrl: './account-shell.component.html',
  styleUrl: './account-shell.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class AccountShellComponent implements OnInit {
  private readonly accountApi = inject(AccountApiService);

  readonly activeItem = input<AccountNavItemId | null>(null);
  readonly showShippingInfo = input<boolean>(false);

  readonly userName = signal<string>(this.readStoredUserName() || 'My Account');
  readonly userEmail = signal<string>('');
  readonly initials = computed(() => buildInitials(this.userName()));

  ngOnInit(): void {
    this.accountApi.me().subscribe({
      next: (user) => {
        const name = user.name?.trim();
        if (name) {
          this.userName.set(name);
        }
        const email = user.email?.trim();
        if (email) {
          this.userEmail.set(email);
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

  readonly navItems: AccountNavItem[] = [
    // TODO(member-04): /orders is owned by Member 04's buyer orders page.
    { id: 'orders', label: 'Orders', route: '/orders', icon: 'box' },
    { id: 'payments', label: 'Payments', route: '/payments/history', icon: 'card' },
    // Route arrives in Step 9.
    { id: 'refunds', label: 'Returns & Refunds', route: '/payments/refunds', icon: 'refresh' },
    // TODO: Addresses / Notifications / Account Settings pages are not built yet.
    { id: 'addresses', label: 'Addresses', route: null, icon: 'pin' },
    { id: 'notifications', label: 'Notifications', route: null, icon: 'bell' },
    { id: 'settings', label: 'Account Settings', route: null, icon: 'user' },
  ];

  isActive(id: AccountNavItemId): boolean {
    const active = this.activeItem();
    if (id === 'refunds' && (active === 'refunds' || active === 'returns')) {
      return true;
    }
    return active === id;
  }
}
