import { createContext, useContext } from "react";
import { translate, type Language } from "./i18n";

export interface I18nContextValue {
  language: Language;
  setLanguage: (language: Language) => void;
  t: (key: string) => string;
}

export const I18nContext = createContext<I18nContextValue>({
  language: "en",
  setLanguage: () => {},
  t: (key: string) => key,
});

export function useI18n() {
  return useContext(I18nContext);
}

export function makeT(language: Language) {
  return (key: string) => translate(language, key);
}