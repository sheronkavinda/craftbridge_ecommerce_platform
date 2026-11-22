export interface AuthUser {
  id: number;
  name: string;
  email: string;
  roles: string[];
}

export interface AuthResponse {
  access_token: string;
  expires_in: number;
  user: AuthUser;
}

export interface Address {
  id?: number;
  label: string;
  full_name: string;
  phone: string;
  address_line1: string;
  address_line2: string;
  city: string;
  state: string;
  postal_code: string;
  country_code: string;
  is_default: boolean;
}
