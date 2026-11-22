import { HttpClient } from '@angular/common/http';
import { inject, Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';
import { Address, AuthResponse, AuthUser } from './account.models';

@Injectable({ providedIn: 'root' })
export class AccountApiService {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = `${environment.apiBaseUrl}/auth`;

  register(input: { name: string; email: string; password: string }): Observable<AuthResponse> {
    return this.http.post<AuthResponse>(`${this.baseUrl}/register`, input);
  }

  login(input: { email: string; password: string }): Observable<AuthResponse> {
    return this.http.post<AuthResponse>(`${this.baseUrl}/login`, input);
  }

  logout(): Observable<void> {
    return this.http.post<void>(`${this.baseUrl}/logout`, {});
  }

  forgotPassword(email: string): Observable<{ message: string }> {
    return this.http.post<{ message: string }>(`${this.baseUrl}/forgot-password`, { email });
  }

  resetPassword(token: string, newPassword: string): Observable<{ message: string }> {
    return this.http.post<{ message: string }>(`${this.baseUrl}/reset-password`, {
      token,
      new_password: newPassword,
    });
  }

  me(): Observable<AuthUser> {
    return this.http.get<AuthUser>(`${this.baseUrl}/me`);
  }

  updateProfile(name: string): Observable<{ message: string }> {
    return this.http.patch<{ message: string }>(`${environment.apiBaseUrl}/me`, { name });
  }

  listAddresses(): Observable<{ addresses: Address[] }> {
    return this.http.get<{ addresses: Address[] }>(`${environment.apiBaseUrl}/me/addresses`);
  }

  createAddress(address: Address): Observable<Address> {
    return this.http.post<Address>(`${environment.apiBaseUrl}/me/addresses`, address);
  }

  updateAddress(id: number, address: Address): Observable<Address> {
    return this.http.patch<Address>(`${environment.apiBaseUrl}/me/addresses/${id}`, address);
  }

  deleteAddress(id: number): Observable<void> {
    return this.http.delete<void>(`${environment.apiBaseUrl}/me/addresses/${id}`);
  }
}
