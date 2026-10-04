import { useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { AlertCircle } from "lucide-react";
import { APIError, AuthUser, authenticate, currentUser } from "./api";
import { AuthUserContext } from "./auth-context";
import { useI18n } from "./i18n-context";
import { Loading } from "./components/EmptyState";

export function AuthGate({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  const [user, setUser] = useState<AuthUser | null>(null);
  const [checking, setChecking] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { currentUser().then(setUser).catch(() => {}).finally(() => setChecking(false)); }, []);
  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(""); setSubmitting(true);
    const data = new FormData(event.currentTarget);
    try { setUser(await authenticate(String(data.get("username")), String(data.get("password")))); }
    catch (cause) { setError(cause instanceof APIError ? cause.message : t("auth.sign_in_failed")); }
    finally { setSubmitting(false); }
  }
  if (checking) return <main className="auth-shell"><div className="auth-card"><Loading>{t("auth.checking")}</Loading></div></main>;
  if (!user) return <main className="auth-shell">
    <span className="auth-brand">
      <img src="/favicon.svg" width="36" height="36" alt="" />
      <span>Velora <span>DNS</span></span>
    </span>
    <form className="auth-card" onSubmit={login}>
      <h1>{t("auth.sign_in_title")}</h1>
      <div className="auth-body">
        {error && <div className="notice error" role="alert"><AlertCircle size={16} />{error}</div>}
        <label>{t("auth.username")}<input name="username" autoComplete="username" required autoFocus /></label>
        <label>{t("auth.password")}<input name="password" type="password" autoComplete="current-password" required /></label>
        <button className="button primary" type="submit" disabled={submitting}>{t("auth.sign_in")}</button>
      </div>
    </form>
  </main>;
  return <AuthUserContext.Provider value={user}>{children}</AuthUserContext.Provider>;
}
