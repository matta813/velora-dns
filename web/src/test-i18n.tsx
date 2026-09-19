import { I18nContext } from "./i18n-context";
import { translate } from "./i18n";

export function withI18n(children: React.ReactNode): React.ReactNode {
  return (
    <I18nContext.Provider
      value={{
        language: "en",
        setLanguage: () => {},
        t: (key: string) => translate("en", key),
      }}
    >
      {children}
    </I18nContext.Provider>
  );
}