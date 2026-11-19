import { InjectionToken } from '@angular/core';

/**
 * Injectable seam for saving a downloaded Blob to the user's device.
 * Replaced with a spy in unit tests so tests never trigger a real browser file download.
 */
export const FILE_SAVER = new InjectionToken<(blob: Blob, filename: string) => void>(
  'FILE_SAVER',
  {
    providedIn: 'root',
    factory: () => (blob: Blob, filename: string) => {
      const objectUrl = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = objectUrl;
      anchor.download = filename;
      anchor.style.display = 'none';
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(objectUrl);
    },
  },
);

/**
 * Injectable seam for triggering the browser print dialog.
 * Replaced with a spy in unit tests so tests never block on window.print().
 */
export const WINDOW_PRINTER = new InjectionToken<() => void>('WINDOW_PRINTER', {
  providedIn: 'root',
  factory: () => () => {
    window.print();
  },
});

export const PRINT = WINDOW_PRINTER;
