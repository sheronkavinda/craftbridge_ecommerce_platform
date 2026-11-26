import { toCents } from './seller.models';
describe('seller product prices', () => {
  it('converts decimal text to exact cents', () => {
    expect(toCents('19.99')).toBe(1999);
    expect(toCents('0.01')).toBe(1);
    expect(toCents('12.5')).toBe(1250);
  });
  it('rejects unrepresentable prices', () => {
    for (const text of ['0', '1.009', '1e2', '', '-1', '999999999999999999999'])
      expect(toCents(text)).toBeNull();
  });
});
