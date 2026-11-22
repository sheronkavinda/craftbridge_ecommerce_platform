import { Routes } from '@angular/router';
import { authGuard } from '../../core/guards/auth.guard';

export const ACCOUNT_ROUTES: Routes = [
  { path: 'login', loadComponent: () => import('./login/login.component').then((m) => m.LoginComponent) },
  { path: 'register', loadComponent: () => import('./register/register.component').then((m) => m.RegisterComponent) },
  { path: 'forgot-password', loadComponent: () => import('./forgot-password/forgot-password.component').then((m) => m.ForgotPasswordComponent) },
  { path: 'reset-password', loadComponent: () => import('./reset-password/reset-password.component').then((m) => m.ResetPasswordComponent) },
  { path: 'account/profile', canActivate: [authGuard], loadComponent: () => import('./profile/profile.component').then((m) => m.ProfileComponent) },
  { path: 'account/addresses', canActivate: [authGuard], loadComponent: () => import('./addresses/addresses.component').then((m) => m.AddressesComponent) },
];
