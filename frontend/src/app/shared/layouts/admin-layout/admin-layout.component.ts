import { CommonModule } from '@angular/common';
import { Component } from '@angular/core';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';

interface AdminNavItem {
  label: string;
  route: string;
}

@Component({
  selector: 'app-admin-layout',
  standalone: true,
  imports: [CommonModule, RouterOutlet, RouterLink, RouterLinkActive],
  templateUrl: './admin-layout.component.html',
  styleUrl: './admin-layout.component.scss',
})
export class AdminLayoutComponent {
  isSidebarOpen = false;

  readonly navItems: AdminNavItem[] = [
    { label: 'Dashboard', route: '/admin/dashboard' },
    { label: 'Seller applications', route: '/admin/seller-approvals' },
    { label: 'Users', route: '/admin/users' },
    { label: 'Categories', route: '/admin/categories' },
    { label: 'Refunds', route: '/admin/refunds' },
    { label: 'Reports', route: '/admin/reports' },
    { label: 'Audit logs', route: '/admin/audit-log' },
  ];

  toggleSidebar(): void {
    this.isSidebarOpen = !this.isSidebarOpen;
  }

  closeSidebar(): void {
    this.isSidebarOpen = false;
  }
}

