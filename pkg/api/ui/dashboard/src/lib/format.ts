// ─── Shared formatting helpers ──────────────────────────────────────
// Centralized number/byte/time formatters used across pages. Keeping
// these in one place ensures consistent output and avoids drift.

/** Format an integer with K/M suffixes for compact display (e.g. 1.2K, 3.4M). */
export function formatNumber(n: number): string {
  if (n == null) return "0";
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

/** Format a byte count with KB/MB/GB suffixes. */
export function formatBytes(bytes: number): string {
  if (bytes == null) return "0 KB";
  if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(1)} GB`;
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(0)} MB`;
  return `${(bytes / 1_024).toFixed(0)} KB`;
}

/** Format an ISO timestamp as 24-hour HH:MM:SS local time. */
export function formatTime(ts: string): string {
  try {
    const d = new Date(ts);
    return d.toLocaleTimeString("en-US", {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hour12: false,
    });
  } catch {
    return ts;
  }
}

/** Format an ISO timestamp as 24-hour HH:MM (no seconds) — useful for chart axes. */
export function formatTimeShort(ts: string): string {
  const d = new Date(ts);
  return d.toLocaleTimeString("en-US", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

/** Format a duration in seconds as a compact uptime string (e.g. "3d 4h 12m"). */
export function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

/** Format a percentage value with one decimal (e.g. 12.3%). */
export function formatPercent(n: number): string {
  if (n == null) return "0%";
  return `${n.toFixed(1)}%`;
}

/** Format a millisecond duration with one decimal and a "<1ms" floor. */
export function formatMs(n: number): string {
  if (n == null) return "0ms";
  if (n < 1) return "<1ms";
  return `${n.toFixed(1)}ms`;
}
