import { useEffect, useState } from "react";
import { request, type AuthUser } from "../api";

export function Users() {
  const [users, setUsers] = useState<AuthUser[]>([]);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [input, setInput] = useState({ username: "", password: "", role: "viewer" as AuthUser["role"] });
  useEffect(() => {
    const controller = new AbortController();
    void request<AuthUser[]>("/api/v1/users", controller.signal).then(setUsers).catch((reason) => { if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "Unable to load users"); });
    return () => controller.abort();
  }, [revision]);
  async function update(id: number, body: object) {
    try { await request(`/api/v1/users/${id}`, undefined, "PUT", { body }); setError(""); setRevision((value) => value + 1); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Unable to update user"); }
  }
  return <>
    {error && <div className="notice error" role="alert">{error}</div>}
    <section className="panel form-panel">
      <form className="zone-form" aria-label="Create user" onSubmit={(event) => { event.preventDefault(); void request("/api/v1/users", undefined, "POST", { body: input }).then(() => { setInput({ username: "", password: "", role: "viewer" }); setRevision((value) => value + 1); }).catch((reason) => setError(reason instanceof Error ? reason.message : "Unable to create user")); }}>
        <div className="form-grid">
          <label>Username<input required minLength={3} maxLength={64} autoComplete="off" value={input.username} onChange={(event) => setInput({ ...input, username: event.target.value })} /></label>
          <label>Temporary password<input required type="password" minLength={12} maxLength={1024} autoComplete="new-password" value={input.password} onChange={(event) => setInput({ ...input, password: event.target.value })} /></label>
          <label>Role<select value={input.role} onChange={(event) => setInput({ ...input, role: event.target.value as AuthUser["role"] })}><option value="viewer">Viewer</option><option value="operator">Operator</option><option value="admin">Admin</option></select></label>
        </div>
        <button className="button primary">Create user</button>
      </form>
    </section>
    <section className="panel"><div className="table-wrap"><table><thead><tr><th>Username</th><th>Role</th><th>Status</th><th>Access</th></tr></thead><tbody>{users.map((user) => <tr key={user.id}><td>{user.username}</td><td><select aria-label={`Role for ${user.username}`} value={user.role} onChange={(event) => void update(user.id, { role: event.target.value })}><option value="viewer">Viewer</option><option value="operator">Operator</option><option value="admin">Admin</option></select></td><td>{user.active ? "Active" : "Disabled"}</td><td><button className="button" onClick={() => void update(user.id, { active: !user.active })}>{user.active ? "Disable" : "Enable"}</button></td></tr>)}</tbody></table></div></section>
  </>;
}
