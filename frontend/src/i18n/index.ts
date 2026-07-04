// i18n init — imported for its side effect once in main.tsx. Keys are stable
// identifiers namespaced by feature; enum labels come from lib/enum-display
// (which returns keys, not strings). Dates/numbers never go through catalogs —
// that's lib/format.ts (Intl).

import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "./locales/en.json";

const stored = typeof localStorage !== "undefined" ? localStorage.getItem("alexandria.locale") : null;

void i18n.use(initReactI18next).init({
    resources: { en: { translation: en } },
    lng: stored ?? undefined, // undefined → i18next falls back to fallbackLng; system-locale detection can come later
    fallbackLng: "en",
    interpolation: { escapeValue: false }, // React already escapes
});

export default i18n;
