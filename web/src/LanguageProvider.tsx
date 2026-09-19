import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { loadPreferences, savePreferences } from "./api";
import { DEFAULT_LANGUAGE, SUPPORTED_LANGUAGES, type Language } from "./i18n";
import { I18nContext, makeT } from "./i18n-context";
import { applyTheme, DEFAULT_THEME, normalizeTheme, type Theme } from "./theme";
import { ThemeContext } from "./theme-context";

function normalizeLanguage(language?: string): Language {
  if (typeof language === "string" && SUPPORTED_LANGUAGES.includes(language as Language)) {
    return language as Language;
  }
  return DEFAULT_LANGUAGE;
}

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguageState] = useState<Language>(
    () =>
      typeof localStorage !== "undefined"
        ? normalizeLanguage(localStorage.getItem("velora_language") ?? undefined)
        : DEFAULT_LANGUAGE,
  );
  const [theme, setThemeState] = useState<Theme>(
    () =>
      typeof localStorage !== "undefined"
        ? normalizeTheme(localStorage.getItem("velora_theme") ?? undefined)
        : DEFAULT_THEME,
  );
  useEffect(() => {
    void loadPreferences()
      .then((prefs) => {
        const nextLanguage = normalizeLanguage(prefs.language);
        const nextTheme = normalizeTheme(prefs.theme);
        setLanguageState(nextLanguage);
        setThemeState(nextTheme);
        if (typeof localStorage !== "undefined") {
          localStorage.setItem("velora_language", nextLanguage);
          localStorage.setItem("velora_theme", nextTheme);
        }
      })
      .catch(() => {});
  }, []);
  useEffect(() => {
    applyTheme(theme);
  }, [theme]);
  function setLanguage(next: Language) {
    setLanguageState(next);
    if (typeof localStorage !== "undefined") {
      localStorage.setItem("velora_language", next);
    }
    void savePreferences({ language: next, theme }).catch(() => {});
  }
  function setTheme(next: Theme) {
    setThemeState(next);
    if (typeof localStorage !== "undefined") {
      localStorage.setItem("velora_theme", next);
    }
    void savePreferences({ language, theme: next }).catch(() => {});
  }
  return (
    <ThemeContext.Provider value={{ theme, setTheme }}>
      <I18nContext.Provider
        value={{ language, setLanguage, t: makeT(language) }}
      >
        {children}
      </I18nContext.Provider>
    </ThemeContext.Provider>
  );
}