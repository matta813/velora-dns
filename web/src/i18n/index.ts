import en from "./en";
import de from "./de";

export const SUPPORTED_LANGUAGES = ["en", "de"] as const;
export type Language = (typeof SUPPORTED_LANGUAGES)[number];
export const DEFAULT_LANGUAGE: Language = "en";

export const dictionaries: Record<Language, Record<string, string>> = {
  en,
  de,
};

export function translate(language: Language, key: string): string {
  const dict = dictionaries[language];
  return dict[key] ?? en[key] ?? key;
}

export function languageName(language: Language): string {
  switch (language) {
    case "de":
      return "Deutsch";
    case "en":
    default:
      return "English";
  }
}
