
import { Routes } from '@angular/router';
import { adminGuard } from './core/guards/admin.guard';
import { ACCOUNT_ROUTES } from './features/account/account.routes';

export const routes: Routes = [
  {
    path: 'cart',
    loadComponent: () => import('./features/cart/cart-page').then(m => m.CartPage),
  },
  ...ACCOUNT_ROUTES,
  // Public Home Page
  {
    path: '',
    pathMatch: 'full',
    loadComponent: () =>
      import('./features/storefront/home/home').then((m) => m.Home),
  },

  // Shop and Search Pageeeee
  
  {
    path: 'shop',
    loadComponent: () =>
      import('./features/storefront/shop/shop').then((m) => m.Shop),
  },


  // Product Detail Page
  {
    path: 'products/:id',
    loadComponent: () =>
      import('./features/storefront/product-detail/product-detail')
        .then((m) => m.ProductDetail),
  },


{
  // Artisan Storefront Page
  path: 'artisans/:id',
  loadComponent: () =>
    import('./features/storefront/artisan-storefront/artisan-storefront')
      .then((m) => m.ArtisanStorefront),
},


  // Wishlist Page
  {
    path: 'wishlist',
    loadComponent: () =>
      import('./features/storefront/wishlist/wishlist')
        .then((m) => m.Wishlist),
  },


{
  path: 'products/:productId/review',
  loadComponent: () =>
    import('./features/storefront/review-editor/review-editor')
      .then((m) => m.ReviewEditor),
},

  // Existing Admin Routes
  {
    path: 'admin',
    canActivate: [adminGuard],
    loadChildren: () =>
      import('./features/admin/admin.routes').then((m) => m.ADMIN_ROUTES),
  },

  // Unknown routes go to Home
  {
    // TODO(member-01): Guard with the real buyer auth guard once Member 01's login exists.
    path: 'payments',
    loadChildren: () =>
      import('./features/payments/payments.routes').then((m) => m.PAYMENTS_ROUTES),
  },
  {
    path: '**',
    redirectTo: '',
  },
];
