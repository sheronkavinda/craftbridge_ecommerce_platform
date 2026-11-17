import { Pipe, PipeTransform } from '@angular/core';

/**
 * Formats integer cents and a currency code without floating-point arithmetic.
 * - USD: "$2,000.00 USD", "$0.05 USD", "$0.00 USD"
 * - Other currencies: "<CUR> 2,000.00"
 */
export function formatMoneyCents(
  amountCents: number | null | undefined,
  currency = 'USD',
): string {
  const safeCents =
    typeof amountCents === 'number' && Number.isFinite(amountCents)
      ? Math.trunc(amountCents)
      : 0;

  const isNegative = safeCents < 0;
  const absCents = Math.abs(safeCents);
  const wholeUnits = Math.trunc(absCents / 100);
  const fractionalCents = absCents % 100;

  const wholeWithCommas = String(wholeUnits).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  const fracTwoDigits = String(fractionalCents).padStart(2, '0');
  const formattedNumber = `${wholeWithCommas}.${fracTwoDigits}`;
  const signPrefix = isNegative ? '-' : '';

  const normalizedCurrency = (currency || 'USD').trim().toUpperCase() || 'USD';
  if (normalizedCurrency === 'USD') {
    return `${signPrefix}$${formattedNumber} USD`;
  }
  return `${signPrefix}${normalizedCurrency} ${formattedNumber}`;
}

@Pipe({
  name: 'money',
  standalone: true,
})
export class MoneyPipe implements PipeTransform {
  transform(amountCents: number | null | undefined, currency = 'USD'): string {
    return formatMoneyCents(amountCents, currency);
  }
}
