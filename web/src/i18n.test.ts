import { describe, expect, it } from "vitest";
import { dictionaries, SUPPORTED_LANGUAGES } from "./i18n";

describe("i18n", () => {
  it("defines the same translation keys for every language", () => {
    const [base, ...rest] = SUPPORTED_LANGUAGES;
    const baseKeys = Object.keys(dictionaries[base]).sort();
    for (const language of rest) {
      expect(Object.keys(dictionaries[language]).sort()).toEqual(baseKeys);
    }
  });

  it("does not ship empty translations", () => {
    for (const language of SUPPORTED_LANGUAGES) {
      const empty = Object.entries(dictionaries[language])
        .filter(([, value]) => value.trim() === "")
        .map(([key]) => key);
      expect(empty).toEqual([]);
    }
  });
});
