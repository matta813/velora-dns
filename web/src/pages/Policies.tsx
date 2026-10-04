import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { Pencil, Plus, RefreshCw, Search, ShieldCheck, Trash2 } from "lucide-react";
import { request, type BlocklistSource } from "../api";
import { loadClients, type ClientView } from "../api-clients";
import {
  createPolicy,
  deletePolicy,
  effectivePolicy,
  loadPolicies,
  updatePolicy,
  type EffectivePolicy,
  type Policy,
  type PolicyInput,
  type PolicyMode,
} from "../api-policies";
import { EmptyState, Loading } from "../components/EmptyState";
import { useI18n } from "../i18n-context";
import "../zones/zones.css";

type Editor = { id: number | null; clientID: number; mode: PolicyMode; blocklists: number[]; allow: string; block: string; enabled: boolean };

const MODE_BADGE: Record<PolicyMode, string> = { default: "badge", disabled: "badge warning", custom: "badge info" };

const domains = (text: string) => text.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean);

export function Policies({ readOnly = false }: { readOnly?: boolean }) {
  const { t } = useI18n();
  const [policies, setPolicies] = useState<Policy[] | null>(null);
  const [clients, setClients] = useState<ClientView[]>([]);
  const [sources, setSources] = useState<BlocklistSource[]>([]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [editor, setEditor] = useState<Editor | null>(null);
  const [lookup, setLookup] = useState("");
  const [effective, setEffective] = useState<EffectivePolicy | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const [nextPolicies, nextClients, nextSources] = await Promise.all([
        loadPolicies(signal),
        loadClients(signal),
        request<BlocklistSource[]>("/api/v1/blocklists", signal),
      ]);
      if (!signal?.aborted) {
        setPolicies(nextPolicies);
        setClients(nextClients.clients);
        setSources(nextSources);
        setError("");
      }
    } catch (e) {
      if (!signal?.aborted) setError(e instanceof Error ? e.message : t("policies.load_failed"));
    }
  }, [t]);

  useEffect(() => {
    const controller = new AbortController();
    const timer = window.setTimeout(() => void load(controller.signal), 0);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [load]);

  async function run(action: () => Promise<unknown>, success = "") {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
      setNotice(success);
      setEffective(null);
      await load();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : t("policies.save_failed"));
      return false;
    } finally {
      setBusy(false);
    }
  }

  const toInput = (form: Editor): PolicyInput => ({
    client_id: form.clientID,
    mode: form.mode,
    blocklists: form.mode === "custom" ? form.blocklists : [],
    allow: form.mode === "custom" ? domains(form.allow) : [],
    block: form.mode === "custom" ? domains(form.block) : [],
    enabled: form.enabled,
  });

  const inputOf = (policy: Policy): PolicyInput => ({
    client_id: policy.client_id, mode: policy.mode, blocklists: policy.blocklists, allow: policy.allow, block: policy.block, enabled: policy.enabled,
  });

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!editor) return;
    const input = toInput(editor);
    const ok = await run(() => (editor.id === null ? createPolicy(input) : updatePolicy(editor.id, input)), t("policies.saved"));
    if (ok) setEditor(null);
  }

  async function check(event: FormEvent) {
    event.preventDefault();
    setError("");
    try {
      setEffective(await effectivePolicy(lookup.trim()));
    } catch (e) {
      setEffective(null);
      setError(e instanceof Error ? e.message : t("policies.load_failed"));
    }
  }

  const clientName = (id: number) => clients.find((client) => client.id === id)?.name ?? `#${id}`;
  const sourceName = (id: number) => sources.find((source) => source.id === id)?.name ?? `#${id}`;
  const available = clients.filter((client) => client.id === editor?.clientID || !policies?.some((policy) => policy.client_id === client.id));
  const modeLabel = (mode: PolicyMode) => t(`policies.mode_${mode}`);

  const startNew = () => {
    const first = clients.find((client) => !policies?.some((policy) => policy.client_id === client.id));
    setEditor({ id: null, clientID: first?.id ?? 0, mode: "custom", blocklists: [], allow: "", block: "", enabled: true });
    setNotice("");
  };

  const summary = (policy: Policy) => {
    if (policy.mode === "disabled") return t("policies.summary_disabled");
    if (policy.mode === "default") return t("policies.summary_default");
    const parts = [policy.blocklists.length ? policy.blocklists.map(sourceName).join(", ") : t("policies.no_lists")];
    if (policy.block.length) parts.push(`+${policy.block.length} ${t("policies.blocked_short")}`);
    if (policy.allow.length) parts.push(`${policy.allow.length} ${t("policies.allowed_short")}`);
    return parts.join(" · ");
  };

  return (
    <>
      <div className="zones-toolbar">
        <form className="policy-lookup" role="search" onSubmit={(event) => void check(event)}>
          <label className="sr-only" htmlFor="policy-lookup">{t("policies.lookup")}</label>
          <input id="policy-lookup" value={lookup} onChange={(e) => setLookup(e.target.value)} placeholder="192.168.1.40" spellCheck={false} autoComplete="off" required />
          <button className="button" type="submit">
            <Search size={15} />
            {t("policies.lookup")}
          </button>
        </form>
        <div>
          <button className="button" disabled={busy} onClick={() => void load()}>
            <RefreshCw size={15} />
            {t("blocklists.reload")}
          </button>
          <button className="button primary" disabled={busy || readOnly || editor !== null || clients.length === 0} onClick={startNew}>
            <Plus size={15} />
            {t("policies.add")}
          </button>
        </div>
      </div>
      {effective && (
        <div className="notice info" role="status">
          {effective.client ? (
            <>
              <strong>{lookup.trim()}</strong> → {effective.client.name}: <span className={MODE_BADGE[effective.mode]}>{modeLabel(effective.mode)}</span>
              {!effective.policy && <> {t("policies.no_policy")}</>}
            </>
          ) : (
            <><strong>{lookup.trim()}</strong> {t("policies.unknown_address")}</>
          )}
        </div>
      )}
      {error && <div className="notice error" role="alert">{error}</div>}
      {notice && <div className="notice success" role="status">{notice}</div>}
      {editor && (
        <section className="panel form-panel">
          <form className="zone-form" aria-label={editor.id === null ? t("policies.add_title") : t("policies.edit_title")} onSubmit={(event) => void save(event)}>
            <div>
              <h3>{editor.id === null ? t("policies.add_title") : t("policies.edit_title")}</h3>
              <p>{t("policies.form_hint")}</p>
            </div>
            <fieldset disabled={busy}>
              <div className="form-grid policy-grid">
                <label>
                  {t("policies.client")}
                  <select value={editor.clientID} onChange={(e) => setEditor({ ...editor, clientID: Number(e.target.value) })} required>
                    {available.map((client) => <option key={client.id} value={client.id}>{client.name}</option>)}
                  </select>
                </label>
                <label>
                  {t("policies.mode")}
                  <select value={editor.mode} onChange={(e) => setEditor({ ...editor, mode: e.target.value as PolicyMode })}>
                    <option value="custom">{t("policies.mode_custom")}</option>
                    <option value="disabled">{t("policies.mode_disabled")}</option>
                    <option value="default">{t("policies.mode_default")}</option>
                  </select>
                  <small>{t(`policies.mode_${editor.mode}_hint`)}</small>
                </label>
              </div>
              {editor.mode === "custom" && (
                <>
                  <fieldset className="policy-lists">
                    <legend>{t("policies.blocklists")}</legend>
                    {sources.length === 0 && <small>{t("policies.no_sources")}</small>}
                    {sources.map((source) => (
                      <label key={source.id} className="check-field">
                        <input
                          type="checkbox"
                          checked={editor.blocklists.includes(source.id)}
                          onChange={(e) => setEditor({ ...editor, blocklists: e.target.checked ? [...editor.blocklists, source.id] : editor.blocklists.filter((id) => id !== source.id) })}
                        />
                        {source.name}
                        {!source.enabled && <small>({t("policies.globally_off")})</small>}
                      </label>
                    ))}
                  </fieldset>
                  <div className="form-grid policy-grid">
                    <label>
                      {t("policies.block")}
                      <textarea rows={4} value={editor.block} onChange={(e) => setEditor({ ...editor, block: e.target.value })} placeholder="games.example" spellCheck={false} />
                      <small>{t("policies.domains_hint")}</small>
                    </label>
                    <label>
                      {t("policies.allow")}
                      <textarea rows={4} value={editor.allow} onChange={(e) => setEditor({ ...editor, allow: e.target.value })} placeholder="school.example" spellCheck={false} />
                      <small>{t("policies.allow_hint")}</small>
                    </label>
                  </div>
                </>
              )}
              <label className="check-field">
                <input type="checkbox" checked={editor.enabled} onChange={(e) => setEditor({ ...editor, enabled: e.target.checked })} />
                {t("policies.enabled")}
              </label>
              <div className="form-actions">
                <button className="button primary" type="submit" disabled={!editor.clientID}>{t("policies.save")}</button>
                <button className="button" type="button" onClick={() => setEditor(null)}>{t("forwarding.cancel")}</button>
              </div>
            </fieldset>
          </form>
        </section>
      )}
      <div className="stack">
        {policies === null && !error && (
          <section className="panel">
            <Loading>{t("app.connecting_panel")}</Loading>
          </section>
        )}
        {policies?.length === 0 && (
          <section className="panel">
            <EmptyState icon={<ShieldCheck size={22} />} title={t("policies.empty_title")}>
              {clients.length === 0 ? <>{t("policies.need_clients")} <Link to="/clients">{t("app.nav.clients")}</Link></> : t("policies.empty_text")}
            </EmptyState>
          </section>
        )}
        {Boolean(policies?.length) && (
          <section className="panel">
            <div className="table-wrap">
              <table>
                <caption className="sr-only">{t("app.title.policies")}</caption>
                <thead>
                  <tr>
                    <th>{t("policies.client")}</th>
                    <th>{t("policies.mode")}</th>
                    <th>{t("policies.details")}</th>
                    <th><span className="sr-only">{t("zones.col_actions")}</span></th>
                  </tr>
                </thead>
                <tbody>
                  {policies?.map((policy) => {
                    const name = clientName(policy.client_id);
                    return (
                      <tr key={policy.id}>
                        <td>
                          <span className="endpoint">
                            <button className="switch" role="switch" aria-checked={policy.enabled} aria-label={`${t("policies.toggle")}: ${name}`} disabled={busy || readOnly} onClick={() => void run(() => updatePolicy(policy.id, { ...inputOf(policy), enabled: !policy.enabled }))} />
                            <Link to={`/clients?name=${encodeURIComponent(name)}`}><strong>{name}</strong></Link>
                          </span>
                        </td>
                        <td>
                          <span className={policy.enabled ? MODE_BADGE[policy.mode] : "badge"}>{policy.enabled ? modeLabel(policy.mode) : t("policies.paused")}</span>
                        </td>
                        <td className="wrap cell-muted">{summary(policy)}</td>
                        <td>
                          <div className="table-actions">
                            <button className="icon-button" disabled={busy || readOnly || editor !== null} title={t("policies.edit")} aria-label={`${t("policies.edit")} ${name}`} onClick={() => {
                              setEditor({ id: policy.id, clientID: policy.client_id, mode: policy.mode, blocklists: policy.blocklists, allow: policy.allow.join("\n"), block: policy.block.join("\n"), enabled: policy.enabled });
                              setNotice("");
                              window.scrollTo({ top: 0 });
                            }}>
                              <Pencil size={15} />
                            </button>
                            <button className="icon-button danger-icon" disabled={busy || readOnly} title={t("policies.delete")} aria-label={`${t("policies.delete")} ${name}`} onClick={() => {
                              if (window.confirm(`${t("policies.delete_confirm")} ${name}?`)) void run(() => deletePolicy(policy.id));
                            }}>
                              <Trash2 size={15} />
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </section>
        )}
      </div>
    </>
  );
}
