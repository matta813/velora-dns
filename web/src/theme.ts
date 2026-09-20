export const SUPPORTED_THEMES = ["light", "dark", "auto"] as const;
export type Theme = (typeof SUPPORTED_THEMES)[number];
export const DEFAULT_THEME: Theme = "auto";

export function normalizeTheme(theme?: string): Theme {
  if (typeof theme === "string" && SUPPORTED_THEMES.includes(theme as Theme)) {
    return theme as Theme;
  }
  return DEFAULT_THEME;
}

// resolveTheme returns the concrete color scheme a preference resolves to.
// "auto" follows the operating system preference.
export function resolveTheme(theme: Theme): "light" | "dark" {
  if (theme === "auto") {
    return typeof window !== "undefined" &&
        window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }
  return theme as "light" | "dark";
}

export function applyTheme(theme: Theme) {
  if (typeof document === "undefined") return;
  document.documentElement.dataset.theme = resolveTheme(theme);
}

export function themeLabel(theme: Theme): string {
  switch (theme) {
    case "light":
      return "Light";
    case "dark":
      return "Dark";
    case "auto":
    default:
      return "Auto";
  }
}