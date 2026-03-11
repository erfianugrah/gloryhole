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
// Lovelace palette — used across charts, badges, and stat cards.

export const STATUS_COLORS = {
  allowed: "#5adecd", // green
  blocked: "#f37e96", // red
  cached: "#8796f4", // blue
  nxdomain: "#f1a171", // peach
} as const;

export const CHART_PALETTE = [
  "#5adecd",
  "#c574dd",
  "#79e6f3",
  "#f1a171",
  "#8796f4",
  "#f37e96",
  "#ffd866",
  "#ff4870",
] as const;

export const CHART_TOOLTIP_STYLE = {
  contentStyle: {
    backgroundColor: "#282a36",
    border: "1px solid #414457",
    borderRadius: "8px",
    fontSize: "12px",
    color: "#fcfcfc",
  },
  itemStyle: { color: "#fcfcfc" },
  labelStyle: { color: "#bdbdc1" },
};
