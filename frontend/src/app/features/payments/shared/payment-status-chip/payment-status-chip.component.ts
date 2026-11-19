import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { PaymentStatus } from '../../payments.models';

@Component({
  selector: 'app-payment-status-chip',
  standalone: true,
  templateUrl: './payment-status-chip.component.html',
  styleUrl: './payment-status-chip.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PaymentStatusChipComponent {
  readonly status = input.required<PaymentStatus>();

  readonly label = computed<string>(() => {
    switch (this.status()) {
      case 'SUCCESS':
        return 'Paid';
      case 'PENDING':
        return 'Pending';
      case 'FAILED':
        return 'Failed';
      case 'CANCELLED':
        return 'Cancelled';
      case 'EXPIRED':
        return 'Expired';
    }
  });

  readonly badgeClasses = computed<string>(() => {
    switch (this.status()) {
      case 'SUCCESS':
        return 'bg-emerald-100 text-emerald-900';
      case 'PENDING':
        return 'bg-amber-100 text-amber-900';
      case 'FAILED':
        return 'bg-red-100 text-red-900';
      case 'CANCELLED':
      case 'EXPIRED':
        return 'bg-slate-200 text-slate-800';
    }
  });
}
