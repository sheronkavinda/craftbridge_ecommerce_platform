import { Pipe, PipeTransform } from '@angular/core';

export function formatCurrencyLkr(value: number | null | undefined): string {
  const numeric = Number(value);
  const safeAmount = Number.isFinite(numeric) ? numeric : 0;
  return (
    'LKR ' +
    safeAmount.toLocaleString('en-US', {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    })
  );
}

@Pipe({
  name: 'currencyLkr',
  standalone: true,
})
export class CurrencyLkrPipe implements PipeTransform {
  transform(value: number | null | undefined): string {
    return formatCurrencyLkr(value);
  }
}

