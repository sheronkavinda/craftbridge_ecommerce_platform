export interface Shop {
  name: string;
  slug: string;
  description: string;
  contact_email: string | null;
  profile_image_url: string | null;
  banner_image_url: string | null;
}
export interface Dashboard {
  shop: Shop;
  total_products: number;
  published_products: number;
  draft_products: number;
  archived_products: number;
  low_stock_products: number;
  available_units: number;
}
export interface ProductDetails {
  category_id: number;
  title: string;
  description: string;
  price_cents: number;
}
export interface Product extends ProductDetails {
  id: number;
  status: 'draft' | 'published' | 'archived';
  currency: string;
  available_qty: number;
  reserved_qty: number;
}
export interface Category {
  id: number;
  name: string;
}
export interface Photo {
  id: number;
  image_url: string;
  alt_text: string;
}
export interface Movement {
  id: number;
  created_at: string;
  reason: string;
  available_delta: number;
  reserved_delta: number;
  actor_user_id: number | null;
}
export function toCents(text: string): number | null {
  const parts = /^(\d+)(?:\.(\d{1,2}))?$/.exec(text.trim());
  if (!parts) return null;
  const cents = Number(parts[1] + (parts[2] ?? '').padEnd(2, '0'));
  return Number.isSafeInteger(cents) && cents > 0 ? cents : null;
}
