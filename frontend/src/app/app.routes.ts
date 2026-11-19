
import { Routes } from '@angular/router';
import { adminGuard } from './core/guards/admin.guard';

export const routes: Routes = [
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
    path: '**',
    redirectTo: '',
  },
];
