import {
  UTF8_BOM,
  buildCsvContent,
  escapeCsvCell,
  formatCsvRangeSuffix,
} from './csv.util';

describe('csv.util', () => {
  it('serializes plain numbers and simple strings without extra quotes', () => {
    expect(escapeCsvCell('2026-11-14')).toBe('2026-11-14');
    expect(escapeCsvCell(12500.5)).toBe('12500.5');
    expect(escapeCsvCell(0)).toBe('0');
    expect(escapeCsvCell(null)).toBe('');
    expect(escapeCsvCell(undefined)).toBe('');
  });

  it('quotes cells containing commas, double quotes, or newlines and escapes internal quotes', () => {
    expect(escapeCsvCell('Amali Handlooms, Colombo')).toBe('"Amali Handlooms, Colombo"');
    expect(escapeCsvCell('Store "Heritage" Crafts')).toBe('"Store ""Heritage"" Crafts"');
    expect(escapeCsvCell('Line 1\nLine 2')).toBe('"Line 1\nLine 2"');
  });

  it('builds CSV content with a leading UTF-8 BOM, header row, and CRLF line breaks', () => {
    const csv = buildCsvContent(
      ['date', 'store_name', 'net_revenue'],
      [
        ['2026-11-13', 'Sunil Wood Crafts', 10000],
        ['2026-11-14', 'Kasun "Clay", Studio', 5200.75],
      ],
    );

    expect(csv.startsWith(UTF8_BOM)).toBeTrue();
    const withoutBom = csv.slice(UTF8_BOM.length);
    expect(withoutBom).toBe(
      'date,store_name,net_revenue\r\n2026-11-13,Sunil Wood Crafts,10000\r\n2026-11-14,"Kasun ""Clay"", Studio",5200.75',
    );
  });

  it('formats date range suffixes for filenames', () => {
    expect(formatCsvRangeSuffix('2026-11-01', '2026-11-14')).toBe('2026-11-01_2026-11-14');
    expect(formatCsvRangeSuffix('', '')).toBe('all-time');
  });
});

