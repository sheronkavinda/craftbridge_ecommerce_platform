import { MoneyPipe, formatMoneyCents } from './money.pipe';

describe('MoneyPipe', () => {
  const pipe = new MoneyPipe();

  it('formats 200000 USD as "$2,000.00 USD"', () => {
    expect(pipe.transform(200000, 'USD')).toBe('$2,000.00 USD');
  });

  it('formats 5 USD as "$0.05 USD"', () => {
    expect(pipe.transform(5, 'USD')).toBe('$0.05 USD');
  });

  it('formats 0 USD as "$0.00 USD"', () => {
    expect(pipe.transform(0, 'USD')).toBe('$0.00 USD');
    expect(pipe.transform(null, 'USD')).toBe('$0.00 USD');
    expect(pipe.transform(undefined)).toBe('$0.00 USD');
  });

  it('formats 11500 USD as "$115.00 USD" (mockup-13 amount)', () => {
    expect(formatMoneyCents(11500, 'USD')).toBe('$115.00 USD');
  });

  it('falls back to "<CUR> 0.00" format for non-USD currencies', () => {
    expect(pipe.transform(200000, 'LKR')).toBe('LKR 2,000.00');
    expect(pipe.transform(0, 'EUR')).toBe('EUR 0.00');
  });
});
