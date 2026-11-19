
import { Component, signal } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';

@Component({
  selector: 'app-header',
  standalone: true,
  imports: [RouterLink, FormsModule],
  templateUrl: './header.html',
  styleUrl: './header.scss',
})
export class Header {
  readonly menuOpen = signal(false);

  searchQuery = '';

  constructor(private router: Router) {}

  searchProducts(): void {
    const query = this.searchQuery.trim();

    if (query) {
      this.menuOpen.set(false);

      this.router.navigate(['/shop'], {
        queryParams: { search: query },
      });
    }
  }

  toggleMenu(): void {
    this.menuOpen.update((open) => !open);
  }

  closeMenu(): void {
    this.menuOpen.set(false);
  }
}
