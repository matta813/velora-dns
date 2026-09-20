import { useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { APIError, AuthUser, authenticate, currentUser } from "./api";
import { AuthUserContext } from "./auth-context";
import { useI18n } from "./i18n-context";

export function AuthGate({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  const [user, setUser] = useState<AuthUser | null>(null);
  const [checking, setChecking] = useState(true);
  const [error, setError] = useState("");
  useEffect(() => { currentUser().then(setUser).catch(() => {}).finally(() => setChecking(false)); }, []);
  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError("");
    const data = new FormData(event.currentTarget);
    try { setUser(await authenticate(String(data.get("username")), String(data.get("password")))); }
    catch (cause) { setError(cause instanceof APIError ? cause.message : t("auth.sign_in_failed")); }
  }
  if (checking) return <main className="auth-shell"><div className="panel padded" role="status">{t("auth.checking")}</div></main>;
  if (!user) return <main className="auth-shell"><form className="panel padded auth-card" onSubmit={login}>
    <img src="/favicon.svg" width="48" height="48" alt="" /><h1>{t("auth.sign_in_title")}</h1>
    {error && <div className="notice error" role="alert">{error}</div>}
    <label>{t("auth.username")}<input name="username" autoComplete="username" required /></label>
    <label>{t("auth.password")}<input name="password" type="password" autoComplete="current-password" required /></label>
    <button className="button" type="submit">{t("auth.sign_in")}</button>
  </form></main>;
  return <AuthUserContext.Provider value={user}>{children}</AuthUserContext.Provider>;
}