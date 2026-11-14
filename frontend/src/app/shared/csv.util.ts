export type CsvPrimitive = string | number | boolean | null | undefined;

export const UTF8_BOM = '\uFEFF';

/**
 * Formats and escapes a single CSV cell according to RFC 4180.
 * Plain numbers are serialized without currency prefixes or thousand separators.
 */
export function escapeCsvCell(value: CsvPrimitive): string {
  if (value === null || value === undefined) {
    return '';
  }

  if (typeof value === 'number') {
    return Number.isFinite(value) ? String(value) : '0';
  }

  const raw = String(value);
  const needsQuotes =
    raw.includes(',') ||
    raw.includes('"') ||
    raw.includes('\n') ||
    raw.includes('\r') ||
    raw.startsWith(' ') ||
    raw.endsWith(' ');

  if (!needsQuotes) {
    return raw;
  }

  return `"${raw.replace(/"/g, '""')}"`;
}

/**
 * Builds a complete CSV string with a leading UTF-8 BOM so spreadsheet software
 * (such as Microsoft Excel) opens UTF-8 characters properly.
 */
export function buildCsvContent(
  headers: readonly string[],
  rows: ReadonlyArray<ReadonlyArray<CsvPrimitive>>,
  includeBom = true,
): string {
  const headerLine = headers.map((h) => escapeCsvCell(h)).join(',');
  const dataLines = rows.map((row) => row.map((cell) => escapeCsvCell(cell)).join(','));
  const body = [headerLine, ...dataLines].join('\r\n');
  return includeBom ? `${UTF8_BOM}${body}` : body;
}

/**
 * Builds a clean filename date suffix (`<from>_<to>` or `all-time`).
 */
export function formatCsvRangeSuffix(startDate = '', endDate = ''): string {
  const from = startDate.trim();
  const to = endDate.trim();
  if (from && to) {
    return `${from}_${to}`;
  }
  if (from) {
    return `${from}_present`;
  }
  if (to) {
    return `start_${to}`;
  }
  return 'all-time';
}

/**
 * Triggers a browser file download for a generated CSV string.
 */
export function downloadCsvFile(filename: string, csvContent: string): void {
  if (typeof document === 'undefined' || typeof Blob === 'undefined') {
    return;
  }

  const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.setAttribute('download', filename);
  link.style.display = 'none';
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}

