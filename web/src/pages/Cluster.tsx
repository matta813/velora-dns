import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Check, Copy, KeyRound, LogOut, Network, Plus, RefreshCw, Server, Trash2, Unplug } from "lucide-react";
import {
  createCluster,
  dissolveCluster,
  issueJoinToken,
  joinCluster,
  leaveCluster,
  loadClusterOverview,
  loadConfigVersions,
  removeClusterMember,
  syncCluster,
  type ClusterMember,
  type ClusterOverview,
  type ConfigVersion,
  type MemberStatus,
} from "../api-cluster";
import { useI18n } from "../i18n-context";
import { Stat } from "../components/Stat";
import { Badge } from "../components/Badge";
import { EmptyState, Loading } from "../components/EmptyState";
import "../zones/zones.css";

const REFRESH_MS = 15_000;

const STATUS_TONE: Record<MemberStatus, "success" | "warning" | "danger" | "info"> = {
  in_sync: "success",
  behind: "info",
  never_synced: "warning",
  stale: "danger",
  error: "danger",
};

export function Cluster({ canManage = false }: { canManage?: boolean }) {
  const { t, language } = useI18n();
  const [overview, setOverview] = useState<ClusterOverview | null>(null);
  const [versions, setVersions] = useState<ConfigVersion[]>([]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [token, setToken] = useState<{ token: string; expires_at: string } | null>(null);
  const [copied, setCopied] = useState(false);

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const [next, history] = await Promise.all([loadClusterOverview(signal), loadConfigVersions(20, signal).catch(() => [])]);
      if (!signal?.aborted) {
        setOverview(next);
        setVersions(history);
        setError("");
      }
    } catch (e) {
      if (!signal?.aborted) setError(e instanceof Error ? e.message : t("cluster.load_failed"));
    }
  }, [t]);

  useEffect(() => {
    const controller = new AbortController();
    const timer = window.setTimeout(() => void load(controller.signal), 0);
    const interval = window.setInterval(() => void load(), REFRESH_MS);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
      window.clearInterval(interval);
    };
  }, [load]);

  async function run(action: () => Promise<ClusterOverview | void>, success = "") {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const next = await action();
      if (next) setOverview(next);
      setNotice(success);
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : t("cluster.action_failed"));
      return false;
    } finally {
      setBusy(false);
    }
  }

  const when = (value: string | null) => (value ? new Date(value).toLocaleString(language) : t("cluster.never"));

  if (!overview) {
    return (
      <section className="panel">
        {error ? <div className="notice error" role="alert">{error}</div> : <Loading>{t("cluster.loading")}</Loading>}
      </section>
    );
  }

  const { state } = overview;
  const inSync = overview.members.filter((member) => member.status === "in_sync").length;
  return (
    <div className="stack">
      <div className="zones-toolbar">
        <span className="zone-count">
          {t(`cluster.role_${state.role}`)}
          {state.node_name && ` · ${state.node_name}`}
        </span>
        <div>
          <button className="button" disabled={busy} onClick={() => void load()}>
            <RefreshCw size={15} />
            {t("blocklists.reload")}
          </button>
        </div>
      </div>
      {error && <div className="notice error" role="alert">{error}</div>}
      {notice && <div className="notice success" role="status">{notice}</div>}

      {state.role !== "standalone" && (
        <div className="stats">
          <Stat label={t("cluster.role")} value={t(`cluster.role_${state.role}`)} note={state.role === "primary" ? state.advertised_url : state.primary_url} tone="info" />
          {state.role === "primary" && <Stat label={t("cluster.replicas")} value={String(overview.members.length)} note={`${inSync} ${t("cluster.in_sync_count")}`} tone={inSync === overview.members.length ? "success" : "warning"} />}
          <Stat label={t("cluster.revision")} value={overview.revision || "—"} note={`${overview.replicated_zones} ${t("cluster.zones_replicated")}`} />
          {state.role === "replica" && <Stat label={t("cluster.last_sync")} value={when(state.last_sync_at)} note={state.last_sync_error ? t("cluster.sync_failed") : t("cluster.sync_ok")} tone={state.last_sync_error ? "danger" : "success"} />}
        </div>
      )}

      {state.role === "standalone" && <StandaloneSetup canManage={canManage} busy={busy} run={run} />}
      {state.role === "primary" && (
        <>
          <section className="panel">
            <div className="panel-heading">
              <div>
                <h2>{t("cluster.members")}</h2>
                <p>{t("cluster.members_hint")}</p>
              </div>
              {canManage && (
                <button className="button primary" disabled={busy} onClick={() => void run(async () => { setToken(await issueJoinToken()); setCopied(false); })}>
                  <Plus size={15} />
                  {t("cluster.add_node")}
                </button>
              )}
            </div>
            {token && (
              <div className="panel-body">
                <div className="notice info join-token" role="status">
                  <KeyRound size={16} />
                  <div>
                    <strong>{t("cluster.token_title")}</strong>
                    <p>{t("cluster.token_steps")}</p>
                    <dl className="restore-facts">
                      <dt>{t("cluster.primary_url")}</dt>
                      <dd><code>{state.advertised_url}</code></dd>
                      <dt>{t("cluster.join_token")}</dt>
                      <dd><code className="token-value">{token.token}</code></dd>
                    </dl>
                    <p>{t("cluster.token_expires")} {new Date(token.expires_at).toLocaleTimeString(language)}. {t("cluster.token_once")}</p>
                    <div className="form-actions">
                      <button className="button small" onClick={() => void navigator.clipboard?.writeText(token.token).then(() => setCopied(true))}>
                        {copied ? <Check size={14} /> : <Copy size={14} />}
                        {copied ? t("cluster.copied") : t("cluster.copy_token")}
                      </button>
                      <button className="button small" onClick={() => setToken(null)}>{t("cluster.token_done")}</button>
                    </div>
                  </div>
                </div>
              </div>
            )}
            {overview.members.length === 0 ? (
              <EmptyState compact icon={<Server size={22} />} title={t("cluster.no_members")}>{t("cluster.no_members_text")}</EmptyState>
            ) : (
              <div className="table-wrap">
                <table>
                  <caption className="sr-only">{t("cluster.members")}</caption>
                  <thead>
                    <tr>
                      <th>{t("cluster.name")}</th>
                      <th>{t("cluster.status")}</th>
                      <th>{t("cluster.version")}</th>
                      <th>{t("cluster.last_seen")}</th>
                      <th><span className="sr-only">{t("zones.col_actions")}</span></th>
                    </tr>
                  </thead>
                  <tbody>
                    {overview.members.map((member) => (
                      <MemberRow key={member.node_id} member={member} canManage={canManage} busy={busy} when={when} onRemove={() => {
                        if (window.confirm(`${t("cluster.remove_confirm")} ${member.name}?`)) void run(() => removeClusterMember(member.node_id), t("cluster.removed"));
                      }} />
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>
          {canManage && (
            <section className="panel danger-zone">
              <div className="panel-heading">
                <div>
                  <h2>{t("cluster.dissolve")}</h2>
                  <p>{t("cluster.dissolve_hint")}</p>
                </div>
                <button className="button outline-danger" disabled={busy} onClick={() => {
                  if (window.confirm(t("cluster.dissolve_confirm"))) void run(dissolveCluster, t("cluster.dissolved"));
                }}>
                  <Unplug size={15} />
                  {t("cluster.dissolve")}
                </button>
              </div>
            </section>
          )}
        </>
      )}
      {state.role === "replica" && (
        <section className="panel">
          <div className="panel-heading">
            <div>
              <h2>{t("cluster.replica_title")}</h2>
              <p>{t("cluster.replica_hint")}</p>
            </div>
            {canManage && (
              <div className="button-group">
                <button className="button" disabled={busy} onClick={() => void run(syncCluster)}>
                  <RefreshCw size={15} className={busy ? "spin" : undefined} />
                  {t("cluster.sync_now")}
                </button>
                <button className="button outline-danger" disabled={busy} onClick={() => {
                  if (window.confirm(t("cluster.leave_confirm"))) void run(leaveCluster, t("cluster.left"));
                }}>
                  <LogOut size={15} />
                  {t("cluster.leave")}
                </button>
              </div>
            )}
          </div>
          <div className="panel-body stack-sm">
            {state.last_sync_error ? (
              <div className="notice error" role="alert">{state.last_sync_error}</div>
            ) : (
              <div className="notice success" role="status">{t("cluster.replica_ok")}</div>
            )}
            <dl className="restore-facts">
              <dt>{t("cluster.primary_url")}</dt>
              <dd><code>{state.primary_url}</code></dd>
              <dt>{t("cluster.cluster_id")}</dt>
              <dd><code>{state.cluster_id}</code></dd>
              <dt>{t("cluster.node_id")}</dt>
              <dd><code>{state.node_id}</code></dd>
            </dl>
          </div>
        </section>
      )}

      <details className="panel cli-restore">
        <summary className="panel-body">{t("cluster.config_history")}</summary>
        {versions.length === 0 ? (
          <EmptyState compact title={t("cluster.no_versions")} />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("cluster.version")}</th>
                  <th>{t("cluster.hash")}</th>
                  <th>{t("cluster.applied_by")}</th>
                  <th>{t("cluster.applied_at")}</th>
                </tr>
              </thead>
              <tbody>
                {versions.map((version) => (
                  <tr key={version.version}>
                    <td className="mono">{version.version}</td>
                    <td><code>{version.config_hash.slice(0, 12)}</code></td>
                    <td>{version.applied_by}</td>
                    <td className="cell-muted">{new Date(version.applied_at).toLocaleString(language)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </details>
    </div>
  );
}

function MemberRow({ member, canManage, busy, when, onRemove }: { member: ClusterMember; canManage: boolean; busy: boolean; when: (value: string | null) => string; onRemove: () => void }) {
  const { t } = useI18n();
  return (
    <tr>
      <td>
        <strong>{member.name}</strong>
        <small className="cell-muted block">{member.address || member.node_id}</small>
      </td>
      <td className="wrap">
        <Badge tone={STATUS_TONE[member.status]}>{t(`cluster.status_${member.status}`)}</Badge>
        {member.last_error && <span className="error-text">{member.last_error}</span>}
      </td>
      <td className="mono">{member.version || "—"}</td>
      <td className="cell-muted">{when(member.last_seen_at)}</td>
      <td>
        {canManage && (
          <div className="table-actions">
            <button className="icon-button danger-icon" disabled={busy} title={t("cluster.remove")} aria-label={`${t("cluster.remove")} ${member.name}`} onClick={onRemove}>
              <Trash2 size={15} />
            </button>
          </div>
        )}
      </td>
    </tr>
  );
}

function StandaloneSetup({ canManage, busy, run }: { canManage: boolean; busy: boolean; run: (action: () => Promise<ClusterOverview | void>, success?: string) => Promise<boolean> }) {
  const { t } = useI18n();
  const origin = typeof window === "undefined" ? "" : window.location.origin;
  const [create, setCreate] = useState({ name: "", advertised_url: origin, allow_insecure: origin.startsWith("http:"), skip_check: false });
  const [join, setJoin] = useState({ primary_url: "", token: "", name: "", allow_insecure: false });

  const submitCreate = (event: FormEvent) => {
    event.preventDefault();
    void run(() => createCluster(create), t("cluster.created"));
  };
  const submitJoin = (event: FormEvent) => {
    event.preventDefault();
    if (window.confirm(t("cluster.join_confirm"))) void run(() => joinCluster(join), t("cluster.joined"));
  };

  if (!canManage) {
    return (
      <section className="panel">
        <EmptyState icon={<Network size={22} />} title={t("cluster.standalone_title")}>{t("cluster.standalone_viewer")}</EmptyState>
      </section>
    );
  }
  return (
    <>
      <div className="notice info" role="note">{t("cluster.scope_note")}</div>
      <div className="grid-2">
        <section className="panel">
          <form className="zone-form" aria-label={t("cluster.create_title")} onSubmit={submitCreate}>
            <div>
              <h3>{t("cluster.create_title")}</h3>
              <p>{t("cluster.create_hint")}</p>
            </div>
            <fieldset disabled={busy}>
              <label>
                {t("cluster.node_name")}
                <input value={create.name} onChange={(e) => setCreate({ ...create, name: e.target.value })} placeholder="dns1" maxLength={64} required />
              </label>
              <label>
                {t("cluster.advertised_url")}
                <input type="url" value={create.advertised_url} onChange={(e) => setCreate({ ...create, advertised_url: e.target.value })} placeholder="https://dns1.example.lan" spellCheck={false} required />
                <small>{t("cluster.advertised_hint")}</small>
              </label>
              <label className="check-field">
                <input type="checkbox" checked={create.allow_insecure} onChange={(e) => setCreate({ ...create, allow_insecure: e.target.checked })} />
                {t("cluster.allow_http")}
              </label>
              <label className="check-field">
                <input type="checkbox" checked={create.skip_check} onChange={(e) => setCreate({ ...create, skip_check: e.target.checked })} />
                {t("cluster.skip_check")}
              </label>
              <div className="form-actions">
                <button className="button primary" type="submit">
                  <Server size={15} />
                  {t("cluster.create")}
                </button>
              </div>
            </fieldset>
          </form>
        </section>
        <section className="panel">
          <form className="zone-form" aria-label={t("cluster.join_title")} onSubmit={submitJoin}>
            <div>
              <h3>{t("cluster.join_title")}</h3>
              <p>{t("cluster.join_hint")}</p>
            </div>
            <fieldset disabled={busy}>
              <label>
                {t("cluster.primary_url")}
                <input type="url" value={join.primary_url} onChange={(e) => setJoin({ ...join, primary_url: e.target.value })} placeholder="https://dns1.example.lan" spellCheck={false} required />
              </label>
              <label>
                {t("cluster.join_token")}
                <input value={join.token} onChange={(e) => setJoin({ ...join, token: e.target.value })} placeholder="vjt_…" spellCheck={false} autoComplete="off" required />
              </label>
              <label>
                {t("cluster.node_name")}
                <input value={join.name} onChange={(e) => setJoin({ ...join, name: e.target.value })} placeholder="dns2" maxLength={64} required />
              </label>
              <label className="check-field">
                <input type="checkbox" checked={join.allow_insecure} onChange={(e) => setJoin({ ...join, allow_insecure: e.target.checked })} />
                {t("cluster.allow_http")}
              </label>
              <p className="notice warning">{t("cluster.join_warning")}</p>
              <div className="form-actions">
                <button className="button" type="submit">
                  <Network size={15} />
                  {t("cluster.join")}
                </button>
              </div>
            </fieldset>
          </form>
        </section>
      </div>
    </>
  );
}
