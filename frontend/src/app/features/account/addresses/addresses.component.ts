import { Component, inject, OnInit } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { AccountApiService } from '../account-api.service';
import { Address } from '../account.models';

@Component({
  standalone: true,
  selector: 'app-addresses',
  imports: [ReactiveFormsModule, RouterLink],
  templateUrl: './addresses.component.html',
})
export class AddressesComponent implements OnInit {
  private readonly fb = inject(FormBuilder);
  private readonly api = inject(AccountApiService);
  addresses: Address[] = [];
  loading = true;
  saving = false;
  error = '';
  message = '';
  selectedAddressId: number | null = null;
  form = this.fb.nonNullable.group({
    label: ['Home', Validators.maxLength(60)],
    full_name: ['', [Validators.required, Validators.maxLength(120)]],
    phone: ['', Validators.maxLength(30)],
    address_line1: ['', [Validators.required, Validators.maxLength(180)]],
    address_line2: ['', Validators.maxLength(180)],
    city: ['', [Validators.required, Validators.maxLength(100)]],
    state: ['', [Validators.required, Validators.maxLength(100)]],
    postal_code: ['', [Validators.required, Validators.maxLength(30)]],
    country_code: ['US', [Validators.required, Validators.pattern(/^[A-Za-z]{2}$/)]],
    is_default: [true],
  });

  ngOnInit(): void { this.load(); }

  load(): void {
    this.loading = true;
    this.error = '';
    this.api.listAddresses().subscribe({
      next: (response) => {
        this.addresses = response.addresses;
        if (this.addresses.length === 0 && this.selectedAddressId === null) {
          this.form.controls.is_default.setValue(true);
        }
        this.loading = false;
      },
      error: () => {
        this.error = 'Your saved addresses could not be loaded.';
        this.loading = false;
      },
    });
  }

  submit(): void {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    this.saving = true;
    this.error = '';
    this.message = '';
    const address = this.form.getRawValue();
    const request = this.selectedAddressId
      ? this.api.updateAddress(this.selectedAddressId, address)
      : this.api.createAddress(address);
    request.subscribe({
      next: () => {
        this.message = this.selectedAddressId ? 'Address updated.' : 'Address saved.';
        this.cancelEdit();
        this.saving = false;
        this.load();
      },
      error: () => {
        this.error = 'The address could not be saved.';
        this.saving = false;
      },
    });
  }

  edit(address: Address): void {
    if (!address.id) return;
    this.selectedAddressId = address.id;
    this.form.reset({
      label: address.label || 'Home',
      full_name: address.full_name,
      phone: address.phone,
      address_line1: address.address_line1,
      address_line2: address.address_line2,
      city: address.city,
      state: address.state,
      postal_code: address.postal_code,
      country_code: address.country_code,
      is_default: address.is_default,
    });
    this.message = '';
  }

  cancelEdit(): void {
    this.selectedAddressId = null;
    this.form.reset({
      label: 'Home',
      full_name: '',
      phone: '',
      address_line1: '',
      address_line2: '',
      city: '',
      state: '',
      postal_code: '',
      country_code: 'US',
      is_default: this.addresses.length === 0,
    });
  }

  remove(address: Address): void {
    if (!address.id) return;
    this.error = '';
    this.message = '';
    this.api.deleteAddress(address.id).subscribe({
      next: () => {
        this.message = 'Address removed.';
        if (this.selectedAddressId === address.id) this.cancelEdit();
        this.load();
      },
      error: () => (this.error = 'The address could not be removed.'),
    });
  }
}
