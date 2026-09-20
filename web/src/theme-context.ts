import { createContext, useContext } from "react";
import type { Theme } from "./theme";

export interface ThemeContextValue {
  theme: Theme;
  setTheme: (theme: Theme) => void;
}

export const ThemeContext = createContext<ThemeContextValue>({
  theme: "auto",
  setTheme: () => {},
});

export function useTheme() {
  return useContext(ThemeContext);
}