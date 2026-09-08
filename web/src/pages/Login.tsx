import { useState } from "react";
import { request, type AuthSession } from "../api";

export function Login({ error: initialError, onLogin }: { error?: string; onLogin: (session: AuthSession) => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState(initialError ?? "");
  const [busy, setBusy] = useState(false);
  return (
    <main className="login-shell">
      <form className="panel login-panel" aria-label="Management login" onSubmit={(event) => {
        event.preventDefault(); setBusy(true); setError("");
        void request<AuthSession>("/api/v1/auth/login", undefined, "POST", { body: { username, password } })
          .then(onLogin).catch((reason) => setError(reason instanceof Error ? reason.message : "Login failed"))
          .finally(() => setBusy(false));
      }}>
        <img src="/favicon.svg" width="48" height="48" alt="" />
        <h1>Sign in to Velora DNS</h1>
        <p>Management access requires an authenticated session.</p>
        {error && <div className="notice error" role="alert">{error}</div>}
        <label>Username<input autoComplete="username" required minLength={3} maxLength={64} value={username} onChange={(event) => setUsername(event.target.value)} /></label>
        <label>Password<input type="password" autoComplete="current-password" required minLength={12} maxLength={1024} value={password} onChange={(event) => setPassword(event.target.value)} /></label>
        <button className="button primary" disabled={busy}>{busy ? "Signing in…" : "Sign in"}</button>
      </form>
    </main>
  );
}
