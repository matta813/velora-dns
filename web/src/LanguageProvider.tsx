import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { loadPreferences, savePreferences } from "./api";
import { DEFAULT_LANGUAGE, SUPPORTED_LANGUAGES, type Language } from "./i18n";
import { I18nContext, makeT } from "./i18n-context";

function normalize(language?: string): Language {
  if (typeof language === "string" && SUPPORTED_LANGUAGES.includes(language as Language)) {
    return language as Language;
  }
  return DEFAULT_LANGUAGE;
}

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguageState] = useState<Language>(
    () =>
      typeof localStorage !== "undefined"
        ? normalize(localStorage.getItem("velora_language") ?? undefined)
        : DEFAULT_LANGUAGE,
  );
  useEffect(() => {
    void loadPreferences()
      .then((prefs) => {
        const next = normalize(prefs.language);
        setLanguageState(next);
        if (typeof localStorage !== "undefined") {
          localStorage.setItem("velora_language", next);
        }
      })
      .catch(() => {});
  }, []);
  function setLanguage(next: Language) {
    setLanguageState(next);
    if (typeof localStorage !== "undefined") {
      localStorage.setItem("velora_language", next);
    }
    void savePreferences({ language: next }).catch(() => {});
  }
  return (
    <I18nContext.Provider
      value={{ language, setLanguage, t: makeT(language) }}
    >
      {children}
    </I18nContext.Provider>
  );
}