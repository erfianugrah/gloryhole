import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/** Safely copy text to the clipboard, logging on failure. */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch (e: unknown) {
    console.error("Clipboard write failed:", e);
    return false;
  }
}

// ─── Centralized Color Map ───────────────────────────────────────────
// Surveillance Terminal palette — used across charts, badges, and stat cards.

export const STATUS_COLORS = {
  allowed: "#00ff41", // green
  blocked: "#ff006e", // pink
  cached: "#4a6cf7", // blue
  nxdomain: "#ff9500", // orange
} as const;

export const CHART_PALETTE = [
  "#00ff41",
  "#ff006e",
  "#4a6cf7",
  "#ffd866",
  "#00d4ff",
  "#ff9500",
  "#00cc33",
  "#cc0058",
] as const;

export const CHART_TOOLTIP_STYLE = {
  contentStyle: {
    backgroundColor: "#141830",
    border: "1px solid #2a2f52",
    borderRadius: "8px",
    fontSize: "12px",
    color: "#e0e0e0",
  },
  itemStyle: { color: "#e0e0e0" },
  labelStyle: { color: "#8888aa" },
};
