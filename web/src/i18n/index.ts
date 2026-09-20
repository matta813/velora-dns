import en from "./en";
import de from "./de";
import es from "./es";
import ptBR from "./pt-BR";
import nl from "./nl";
import pl from "./pl";
import cs from "./cs";
import sv from "./sv";
import ja from "./ja";
import ko from "./ko";

export const SUPPORTED_LANGUAGES = ["en", "de", "es", "pt-BR", "nl", "pl", "cs", "sv", "ja", "ko"] as const;
export type Language = (typeof SUPPORTED_LANGUAGES)[number];
export const DEFAULT_LANGUAGE: Language = "en";

export const dictionaries: Record<Language, Record<string, string>> = {
  en,
  de,
  es,
  "pt-BR": ptBR,
  nl,
  pl,
  cs,
  sv,
  ja,
  ko,
};

export function translate(language: Language, key: string): string {
  const dict = dictionaries[language];
  return dict[key] ?? en[key] ?? key;
}

export function languageName(language: Language): string {
  switch (language) {
    case "de":
      return "Deutsch";
    case "es":
      return "Español";
    case "pt-BR":
      return "Português (Brasil)";
    case "nl":
      return "Nederlands";
    case "pl":
      return "Polski";
    case "cs":
      return "Čeština";
    case "sv":
      return "Svenska";
    case "ja":
      return "日本語";
    case "ko":
      return "한국어";
    case "en":
    default:
      return "English";
  }
}
