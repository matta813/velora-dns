import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { MonitorSmartphone, Pencil, Plus, RefreshCw, Tag, Trash2 } from "lucide-react";
import {
  createClient,
  deleteClient,
  loadClients,
  updateClient,
  type ClientInput,
  type ClientList,
  type ClientView,
} from "../api-clients";
import { EmptyState, Loading } from "../components/EmptyState";
import { useI18n } from "../i18n-context";
import "../zones/zones.css";

type Editor = { id: number | null; name: string; addresses: string; group: string; description: string; enabled: boolean };

const blank: Editor = { id: null, name: "", addresses: "", group: "", description: "", enabled: true };

export function Clients({ readOnly = false, queryLogging = true }: { readOnly?: boolean; queryLogging?: boolean }) {
  const { t, language } = useI18n();
  const [params] = useSearchParams();
  const highlight = params.get("name") ?? "";
  const [data, setData] = useState<ClientList | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  // ?new=1 opens the add form (quick action in the command palette).
  const [editor, setEditor] = useState<Editor | null>(() => (params.get("new") === "1" && !readOnly ? { ...blank } : null));

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const next = await loadClients(signal);
      if (!signal?.aborted) {
        setData(next);
        setError("");
      }
    } catch (e) {
      if (!signal?.aborted) setError(e instanceof Error ? e.message : t("clients.load_failed"));
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

  useEffect(() => {
    if (!highlight || !data) return;
    document.getElementById(`client-${encodeURIComponent(highlight)}`)?.scrollIntoView({ block: "center" });
  }, [highlight, data]);

  const toInput = (form: Editor): ClientInput => ({
    name: form.name.trim(),
    addresses: form.addresses.split(/[\s,]+/).filter(Boolean),
    group: form.group.trim(),
    description: form.description.trim(),
    enabled: form.enabled,
  });

  async function run(action: () => Promise<unknown>, success = "") {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
      setNotice(success);
      await load();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : t("clients.save_failed"));
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!editor) return;
    const input = toInput(editor);
    const ok = await run(() => (editor.id === null ? createClient(input) : updateClient(editor.id, input)), t("clients.saved"));
    if (ok) setEditor(null);
  }

  const edit = (client: ClientView) => {
    setEditor({ id: client.id, name: client.name, addresses: client.addresses.join(", "), group: client.group, description: client.description, enabled: client.enabled });
    setNotice("");
    window.scrollTo({ top: 0 });
  };

  const inputOf = (client: ClientView): ClientInput => ({
    name: client.name, addresses: client.addresses, group: client.group, description: client.description, enabled: client.enabled,
  });

  const clients = data?.clients;
  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">{clients ? `${clients.length} ${t("clients.count")}` : ""}</span>
        <div>
          <button className="button" disabled={busy} onClick={() => void load()}>
            <RefreshCw size={15} />
            {t("blocklists.reload")}
          </button>
          <button className="button primary" disabled={busy || readOnly || editor !== null} onClick={() => { setEditor({ ...blank }); setNotice(""); }}>
            <Plus size={15} />
            {t("clients.add")}
          </button>
        </div>
      </div>
      {error && <div className="notice error" role="alert">{error}</div>}
      {notice && <div className="notice success" role="status">{notice}</div>}
      {editor && (
        <section className="panel form-panel">
          <form className="zone-form" aria-label={editor.id === null ? t("clients.add_title") : t("clients.edit_title")} onSubmit={(event) => void save(event)}>
            <div>
              <h3>{editor.id === null ? t("clients.add_title") : t("clients.edit_title")}</h3>
              <p>{t("clients.form_hint")}</p>
            </div>
            <fieldset disabled={busy}>
              <div className="form-grid client-grid">
                <label>
                  {t("clients.name")}
                  <input value={editor.name} onChange={(e) => setEditor({ ...editor, name: e.target.value })} placeholder="Living Room TV" maxLength={80} required autoFocus />
                </label>
                <label>
                  {t("clients.addresses")}
                  <input value={editor.addresses} onChange={(e) => setEditor({ ...editor, addresses: e.target.value })} placeholder="192.168.1.20, 2001:db8::20" spellCheck={false} autoComplete="off" required />
                  <small>{t("clients.addresses_hint")}</small>
                </label>
                <label>
                  {t("clients.group")}
                  <input value={editor.group} onChange={(e) => setEditor({ ...editor, group: e.target.value })} placeholder="Media" maxLength={60} />
                </label>
                <label>
                  {t("clients.description")}
                  <input value={editor.description} onChange={(e) => setEditor({ ...editor, description: e.target.value })} maxLength={200} />
                </label>
              </div>
              <label className="check-field">
                <input type="checkbox" checked={editor.enabled} onChange={(e) => setEditor({ ...editor, enabled: e.target.checked })} />
                {t("clients.enabled")}
              </label>
              <div className="form-actions">
                <button className="button primary" type="submit">{t("clients.save")}</button>
                <button className="button" type="button" onClick={() => setEditor(null)}>{t("forwarding.cancel")}</button>
              </div>
            </fieldset>
          </form>
        </section>
      )}
      {!queryLogging && <div className="notice info" role="status">{t("clients.logging_off")}</div>}
      <div className="stack">
        {data === null && !error && (
          <section className="panel">
            <Loading>{t("app.connecting_panel")}</Loading>
          </section>
        )}
        {clients?.length === 0 && (
          <section className="panel">
            <EmptyState icon={<MonitorSmartphone size={22} />} title={t("clients.empty_title")}>
              {t("clients.empty_text")}
            </EmptyState>
          </section>
        )}
        {Boolean(clients?.length) && (
          <section className="panel">
            <div className="table-wrap">
              <table>
                <caption className="sr-only">{t("app.title.clients")}</caption>
                <thead>
                  <tr>
                    <th>{t("clients.name")}</th>
                    <th>{t("clients.addresses")}</th>
                    <th>{t("clients.activity")}</th>
                    <th>{t("clients.description")}</th>
                    <th><span className="sr-only">{t("zones.col_actions")}</span></th>
                  </tr>
                </thead>
                <tbody>
                  {clients?.map((client) => {
                    const single = client.addresses.length === 1 && !client.addresses[0].includes("/") ? client.addresses[0] : "";
                    return (
                      <tr key={client.id} id={`client-${encodeURIComponent(client.name)}`} className={client.name === highlight ? "selected" : undefined}>
                        <td>
                          <span className="endpoint">
                            <button className="switch" role="switch" aria-checked={client.enabled} aria-label={`${t("clients.toggle")}: ${client.name}`} disabled={busy || readOnly} onClick={() => void run(() => updateClient(client.id, { ...inputOf(client), enabled: !client.enabled }))} />
                            <span>
                              <strong>{client.name}</strong>
                              {client.group && <small><Tag size={11} /> {client.group}</small>}
                            </span>
                          </span>
                        </td>
                        <td className="wrap">
                          {client.addresses.map((address) => <div key={address}><code>{address}</code></div>)}
                        </td>
                        <td className="cell-muted">
                          {client.activity === null ? "—" : client.activity.queries === 0 ? t("clients.no_activity") : (
                            <>
                              <span className="num">{client.activity.queries.toLocaleString(language)}</span> {t("clients.queries")}
                              {client.activity.last_seen && <small>{t("clients.last_seen")}: {new Date(client.activity.last_seen).toLocaleString(language)}</small>}
                              {single && <small><Link to={`/queries?client=${encodeURIComponent(single)}`}>{t("clients.view_queries")}</Link></small>}
                            </>
                          )}
                        </td>
                        <td className="wrap cell-muted">{client.description || "—"}</td>
                        <td>
                          <div className="table-actions">
                            <button className="icon-button" disabled={busy || readOnly || editor !== null} title={t("clients.edit")} aria-label={`${t("clients.edit")} ${client.name}`} onClick={() => edit(client)}>
                              <Pencil size={15} />
                            </button>
                            <button className="icon-button danger-icon" disabled={busy || readOnly} title={t("clients.delete")} aria-label={`${t("clients.delete")} ${client.name}`} onClick={() => {
                              if (window.confirm(`${t("clients.delete_confirm")} ${client.name}?`)) void run(() => deleteClient(client.id));
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
        {Boolean(data?.unnamed.length) && (
          <section className="panel">
            <div className="panel-heading">
              <div>
                <h2>{t("clients.unnamed_title")}</h2>
                <p>{t("clients.unnamed_text")}</p>
              </div>
            </div>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>{t("querylog.col_client")}</th>
                    <th className="num">{t("clients.queries")}</th>
                    <th>{t("clients.last_seen")}</th>
                    <th><span className="sr-only">{t("zones.col_actions")}</span></th>
                  </tr>
                </thead>
                <tbody>
                  {data?.unnamed.map((item) => (
                    <tr key={item.client_ip}>
                      <td><code>{item.client_ip}</code></td>
                      <td className="num mono">{item.queries.toLocaleString(language)}</td>
                      <td className="cell-muted">{new Date(item.last_seen).toLocaleString(language)}</td>
                      <td>
                        <div className="table-actions">
                          <button className="button small" disabled={busy || readOnly || editor !== null} onClick={() => { setEditor({ ...blank, addresses: item.client_ip }); window.scrollTo({ top: 0 }); }}>
                            <Plus size={14} />
                            {t("clients.name_it")}
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        )}
      </div>
    </>
  );
}
