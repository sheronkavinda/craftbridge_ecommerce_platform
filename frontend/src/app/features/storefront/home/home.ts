
import { Component, inject, OnInit } from '@angular/core';
import { RouterLink } from '@angular/router';

import { Header } from '../../../shared/components/header/header';

import {
  ProductCard,
  StorefrontProduct,
} from '../../../shared/components/product-card/product-card';

import { StorefrontApi } from '../services/storefront-api';

@Component({
  selector: 'app-home',
  standalone: true,
  imports: [RouterLink, Header, ProductCard],
  templateUrl: './home.html',
  styleUrl: './home.scss',
})
export class Home implements OnInit {

  // Storefront API Service
  private readonly storefrontApi = inject(StorefrontApi);

  // Featured Products
  // Real product data will be loaded when the Catalog API is ready.
  readonly featuredProducts: StorefrontProduct[] = [];

  // Storefront Categories
  readonly categories = [
    {
      name: 'Jewelry',
      search: 'jewelry',
      image: '/images/category-jewelry.jpg',
    },
    {
      name: 'Home Decor',
      search: 'home decor',
      image: '/images/category-home-decor.jpg',
    },
    {
      name: 'Textiles',
      search: 'textiles',
      image: '/images/category-textiles.jpg',
    },
    {
      name: 'Art',
      search: 'art',
      image: '/images/category-art.jpg',
    },
    {
      name: 'Gifts',
      search: 'gifts',
      image: '/images/category-gifts.jpg',
    },
  ];

  // Runs automatically when Home Page loads
  ngOnInit(): void {
    this.checkStorefrontApi();
  }

  // Check connection between Angular and Go backend
  private checkStorefrontApi(): void {
    this.storefrontApi.getHealth().subscribe({
      next: (response) => {
        console.log('Storefront API connected:', response);
      },
      error: (error) => {
        console.error('Storefront API connection failed:', error);
      },
    });
  }
}
