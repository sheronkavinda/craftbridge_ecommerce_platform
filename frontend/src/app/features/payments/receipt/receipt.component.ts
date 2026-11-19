import { DatePipe } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  OnInit,
  inject,
  signal,
} from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { ReceiptItem, ReceiptView } from '../payments.models';
import { PaymentsApiError, PaymentsApiService } from '../payments-api.service';
import { AccountShellComponent } from '../shared/account-shell/account-shell.component';
import { FILE_SAVER, PRINT } from '../shared/browser-actions.tokens';
import { MoneyPipe } from '../shared/money.pipe';
import { PaymentStatusChipComponent } from '../shared/payment-status-chip/payment-status-chip.component';

@Component({
  selector: 'app-receipt',
  standalone: true,
  imports: [
    DatePipe,
    RouterLink,
    AccountShellComponent,
    MoneyPipe,
    PaymentStatusChipComponent,
  ],
  templateUrl: './receipt.component.html',
  styleUrl: './receipt.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ReceiptComponent implements OnInit {
  private readonly route = inject(ActivatedRoute);
  private readonly paymentsApi = inject(PaymentsApiService);
  private readonly saveFile = inject(FILE_SAVER);
  private readonly printPage = inject(PRINT);

  readonly orderId = signal<string>('');
  readonly loading = signal<boolean>(true);
  readonly errorStatus = signal<number | null>(null);
  readonly errorMessage = signal<string | null>(null);
  readonly receipt = signal<ReceiptView | null>(null);

  readonly downloading = signal<boolean>(false);
  readonly downloadError = signal<string | null>(null);

  ngOnInit(): void {
    const paramOrderId = (this.route.snapshot?.paramMap.get('orderId') ?? '').trim();
    this.orderId.set(paramOrderId);
    if (!paramOrderId) {
      this.loading.set(false);
      this.errorStatus.set(404);
      this.errorMessage.set("We couldn't find a receipt for this order.");
      return;
    }
    this.loadReceipt();
  }

  loadReceipt(): void {
    const currentOrderId = this.orderId();
    if (!currentOrderId) {
      return;
    }

    this.loading.set(true);
    this.errorStatus.set(null);
    this.errorMessage.set(null);

    this.paymentsApi.getReceipt(currentOrderId).subscribe({
      next: (res) => {
        this.receipt.set(res);
        this.loading.set(false);
      },
      error: (err: unknown) => {
        this.loading.set(false);
        const status = err instanceof PaymentsApiError ? err.status : 0;
        this.errorStatus.set(status);
        this.errorMessage.set(this.mapErrorStatusToMessage(status));
      },
    });
  }

  onDownloadReceipt(): void {
    const currentReceipt = this.receipt();
    const currentOrderId = this.orderId();
    if (!currentReceipt || !currentOrderId || this.downloading()) {
      return;
    }

    this.downloading.set(true);
    this.downloadError.set(null);

    this.paymentsApi
      .downloadReceipt(currentOrderId, currentReceipt.receipt_number)
      .subscribe({
        next: ({ blob, filename }) => {
          this.downloading.set(false);
          this.saveFile(blob, filename);
        },
        error: () => {
          this.downloading.set(false);
          this.downloadError.set(
            'Could not download the receipt right now. Please try again or use Print.',
          );
        },
      });
  }

  onPrintReceipt(): void {
    this.printPage();
  }

  itemLineTotalCents(item: ReceiptItem): number {
    if (typeof item.line_total_cents === 'number' && Number.isFinite(item.line_total_cents)) {
      return item.line_total_cents;
    }
    return (item.unit_price_cents || 0) * (item.quantity || 1);
  }

  private mapErrorStatusToMessage(status: number): string {
    switch (status) {
      case 404:
        return "We couldn't find a receipt for this order.";
      case 409:
        return "This order doesn't have a verified payment yet. If you just paid, your payment may still be confirming.";
      default:
        return 'We could not load your receipt right now. Please try again.';
    }
  }
}
