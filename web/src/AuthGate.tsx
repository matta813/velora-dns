import { useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { APIError, AuthUser, authenticate, currentUser } from "./api";
import { AuthUserContext } from "./auth-context";

export function AuthGate({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [checking, setChecking] = useState(true);
  const [error, setError] = useState("");
  useEffect(() => { currentUser().then(setUser).catch(() => {}).finally(() => setChecking(false)); }, []);
  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError("");
    const data = new FormData(event.currentTarget);
    try { setUser(await authenticate(String(data.get("username")), String(data.get("password")))); }
    catch (cause) { setError(cause instanceof APIError ? cause.message : "Sign in failed"); }
  }
  if (checking) return <main className="auth-shell"><div className="panel padded" role="status">Checking session…</div></main>;
  if (!user) return <main className="auth-shell"><form className="panel padded auth-card" onSubmit={login}>
    <img src="/favicon.svg" width="48" height="48" alt="" /><h1>Sign in to Velora DNS</h1>
    {error && <div className="notice error" role="alert">{error}</div>}
    <label>Username<input name="username" autoComplete="username" required /></label>
    <label>Password<input name="password" type="password" autoComplete="current-password" required /></label>
    <button className="button" type="submit">Sign in</button>
  </form></main>;
  return <AuthUserContext.Provider value={user}>{children}</AuthUserContext.Provider>;
}
