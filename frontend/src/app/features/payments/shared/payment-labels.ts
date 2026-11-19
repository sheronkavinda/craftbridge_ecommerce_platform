/** Maps a backend provider code to a human-readable payment method label. */
export function formatProviderLabel(provider: string | null | undefined): string {
  const raw = (provider ?? '').trim();
  const lowered = raw.toLowerCase();
  if (!raw || lowered === 'payhere_sandbox' || lowered === 'payhere') {
    return 'PayHere Sandbox';
  }
  return raw;
}

/** Builds up to two uppercase initials from a display name ("Ruwan Fernando" -> "RF"). */
export function buildInitials(name: string | null | undefined): string {
  const parts = (name ?? '').trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) {
    return '?';
  }
  const first = parts[0].charAt(0);
  const last = parts.length > 1 ? parts[parts.length - 1].charAt(0) : '';
  return (first + last).toUpperCase();
}
