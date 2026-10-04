import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Pencil, Plus, RefreshCw, Send, Trash2, Webhook as WebhookIcon } from "lucide-react";
import {
  createWebhook,
  deleteWebhook,
  loadEventTypes,
  loadWebhooks,
  testWebhook,
  updateWebhook,
  type Severity,
  type Webhook,
  type WebhookInput,
} from "../api-webhooks";
import { EmptyState, Loading } from "../components/EmptyState";
import { useI18n } from "../i18n-context";
import "../zones/zones.css";

type Editor = {
  id: number | null;
  name: string;
  url: string;
  events: string[];
  minSeverity: Severity;
  allowPrivate: boolean;
  enabled: boolean;
  hasToken: boolean;
  // null keeps the stored token; "" (with hasToken) removes it.
  token: string | null;
};

const blank: Editor = { id: null, name: "", url: "", events: [], minSeverity: "info", allowPrivate: false, enabled: true, hasToken: false, token: "" };

export function Webhooks() {
  const { t, language } = useI18n();
  const [hooks, setHooks] = useState<Webhook[] | null>(null);
  const [eventTypes, setEventTypes] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [editor, setEditor] = useState<Editor | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const [next, types] = await Promise.all([loadWebhooks(signal), loadEventTypes(signal)]);
      if (!signal?.aborted) {
        setHooks(next);
        setEventTypes(types);
        setError("");
      }
    } catch (e) {
      if (!signal?.aborted) setError(e instanceof Error ? e.message : t("webhooks.load_failed"));
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
      await load();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : t("webhooks.save_failed"));
      return false;
    } finally {
      setBusy(false);
    }
  }

  const inputOf = (hook: Webhook): WebhookInput => ({
    name: hook.name, url: hook.url, events: hook.events, min_severity: hook.min_severity, allow_private: hook.allow_private, enabled: hook.enabled,
  });

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!editor) return;
    const input: WebhookInput = {
      name: editor.name.trim(),
      url: editor.url.trim(),
      events: editor.events,
      min_severity: editor.minSeverity,
      allow_private: editor.allowPrivate,
      enabled: editor.enabled,
    };
    if (editor.token !== null) input.token = editor.token.trim();
    const ok = await run(() => (editor.id === null ? createWebhook(input) : updateWebhook(editor.id, input)), t("webhooks.saved"));
    if (ok) setEditor(null);
  }

  async function sendTest(hook: Webhook) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await testWebhook(hook.id);
      await load();
      if (result.ok) setNotice(`${t("webhooks.test_ok")} ${hook.name} (${result.webhook.last_status})`);
      else setError(`${t("webhooks.test_failed")} ${hook.name}: ${result.error}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("webhooks.test_failed"));
    } finally {
      setBusy(false);
    }
  }

  const toggleEvent = (type: string, on: boolean) =>
    editor && setEditor({ ...editor, events: on ? [...editor.events, type] : editor.events.filter((item) => item !== type) });

  const status = (hook: Webhook) => {
    if (!hook.last_delivery_at) return <span className="cell-muted">{t("webhooks.never")}</span>;
    const when = new Date(hook.last_delivery_at).toLocaleString(language);
    if (hook.consecutive_failures > 0) {
      return (
        <>
          <span className="badge danger">{t("webhooks.failing")} ×{hook.consecutive_failures}</span>
          <small className="cell-muted" title={hook.last_error}>{hook.last_error}</small>
          <small className="cell-muted">{when}</small>
        </>
      );
    }
    return (
      <>
        <span className="badge success">{hook.last_status}</span>
        <small className="cell-muted">{when}</small>
      </>
    );
  };

  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">{hooks ? `${hooks.length} ${t("webhooks.count")}` : ""}</span>
        <div>
          <button className="button" disabled={busy} onClick={() => void load()}>
            <RefreshCw size={15} />
            {t("blocklists.reload")}
          </button>
          <button className="button primary" disabled={busy || editor !== null} onClick={() => { setEditor({ ...blank }); setNotice(""); }}>
            <Plus size={15} />
            {t("webhooks.add")}
          </button>
        </div>
      </div>
      {error && <div className="notice error" role="alert">{error}</div>}
      {notice && <div className="notice success" role="status">{notice}</div>}
      {editor && (
        <section className="panel form-panel">
          <form className="zone-form" aria-label={editor.id === null ? t("webhooks.add_title") : t("webhooks.edit_title")} onSubmit={(event) => void save(event)}>
            <div>
              <h3>{editor.id === null ? t("webhooks.add_title") : t("webhooks.edit_title")}</h3>
              <p>{t("webhooks.form_hint")}</p>
            </div>
            <fieldset disabled={busy}>
              <div className="form-grid webhook-grid">
                <label>
                  {t("webhooks.name")}
                  <input value={editor.name} onChange={(e) => setEditor({ ...editor, name: e.target.value })} placeholder="Home Assistant" maxLength={80} required autoFocus />
                </label>
                <label>
                  {t("webhooks.url")}
                  <input type="url" value={editor.url} onChange={(e) => setEditor({ ...editor, url: e.target.value })} placeholder="https://hooks.example.com/velora" spellCheck={false} autoComplete="off" required />
                </label>
                <label>
                  {t("webhooks.min_severity")}
                  <select value={editor.minSeverity} onChange={(e) => setEditor({ ...editor, minSeverity: e.target.value as Severity })}>
                    <option value="info">{t("webhooks.severity_info")}</option>
                    <option value="warning">{t("webhooks.severity_warning")}</option>
                    <option value="critical">{t("webhooks.severity_critical")}</option>
                  </select>
                </label>
                <label>
                  {t("webhooks.token")}
                  {editor.token === null ? (
                    <span className="token-kept">
                      <span className="cell-muted">{t("webhooks.token_stored")}</span>
                      <button type="button" className="button small" onClick={() => setEditor({ ...editor, token: "" })}>{t("webhooks.token_change")}</button>
                    </span>
                  ) : (
                    <input type="password" value={editor.token} onChange={(e) => setEditor({ ...editor, token: e.target.value })} autoComplete="new-password" spellCheck={false} placeholder={editor.hasToken ? t("webhooks.token_remove_hint") : ""} />
                  )}
                  <small>{t("webhooks.token_hint")}</small>
                </label>
              </div>
              <fieldset className="policy-lists">
                <legend>{t("webhooks.events")}</legend>
                <small className="events-hint">{t("webhooks.events_hint")}</small>
                {eventTypes.map((type) => (
                  <label key={type} className="check-field">
                    <input type="checkbox" checked={editor.events.includes(type)} onChange={(e) => toggleEvent(type, e.target.checked)} />
                    <code>{type}</code>
                  </label>
                ))}
              </fieldset>
              <label className="check-field">
                <input type="checkbox" checked={editor.allowPrivate} onChange={(e) => setEditor({ ...editor, allowPrivate: e.target.checked })} />
                {t("webhooks.allow_private")}
              </label>
              <label className="check-field">
                <input type="checkbox" checked={editor.enabled} onChange={(e) => setEditor({ ...editor, enabled: e.target.checked })} />
                {t("webhooks.enabled")}
              </label>
              <div className="form-actions">
                <button className="button primary" type="submit">{t("webhooks.save")}</button>
                <button className="button" type="button" onClick={() => setEditor(null)}>{t("forwarding.cancel")}</button>
              </div>
            </fieldset>
          </form>
        </section>
      )}
      <div className="stack">
        {hooks === null && !error && (
          <section className="panel">
            <Loading>{t("app.connecting_panel")}</Loading>
          </section>
        )}
        {hooks?.length === 0 && (
          <section className="panel">
            <EmptyState icon={<WebhookIcon size={22} />} title={t("webhooks.empty_title")}>
              {t("webhooks.empty_text")}
            </EmptyState>
          </section>
        )}
        {Boolean(hooks?.length) && (
          <section className="panel">
            <div className="table-wrap">
              <table>
                <caption className="sr-only">{t("app.title.webhooks")}</caption>
                <thead>
                  <tr>
                    <th>{t("webhooks.name")}</th>
                    <th>{t("webhooks.events")}</th>
                    <th>{t("webhooks.last_delivery")}</th>
                    <th><span className="sr-only">{t("zones.col_actions")}</span></th>
                  </tr>
                </thead>
                <tbody>
                  {hooks?.map((hook) => (
                    <tr key={hook.id}>
                      <td>
                        <span className="endpoint">
                          <button className="switch" role="switch" aria-checked={hook.enabled} aria-label={`${t("webhooks.toggle")}: ${hook.name}`} disabled={busy} onClick={() => void run(() => updateWebhook(hook.id, { ...inputOf(hook), enabled: !hook.enabled }))} />
                          <span>
                            <strong>{hook.name}</strong>
                            <small className="webhook-url"><code>{hook.url}</code></small>
                          </span>
                        </span>
                      </td>
                      <td className="wrap cell-muted">
                        {hook.events.length === 0 ? t("webhooks.all_events") : hook.events.join(", ")}
                        {hook.min_severity !== "info" && <small>{t("webhooks.min_severity")}: {t(`webhooks.severity_${hook.min_severity}`)}</small>}
                      </td>
                      <td className="webhook-status">{status(hook)}</td>
                      <td>
                        <div className="table-actions">
                          <button className="icon-button" disabled={busy} title={t("webhooks.test")} aria-label={`${t("webhooks.test")} ${hook.name}`} onClick={() => void sendTest(hook)}>
                            <Send size={15} />
                          </button>
                          <button className="icon-button" disabled={busy || editor !== null} title={t("webhooks.edit")} aria-label={`${t("webhooks.edit")} ${hook.name}`} onClick={() => {
                            setEditor({ id: hook.id, name: hook.name, url: hook.url, events: hook.events, minSeverity: hook.min_severity, allowPrivate: hook.allow_private, enabled: hook.enabled, hasToken: hook.has_token, token: hook.has_token ? null : "" });
                            setNotice("");
                            window.scrollTo({ top: 0 });
                          }}>
                            <Pencil size={15} />
                          </button>
                          <button className="icon-button danger-icon" disabled={busy} title={t("webhooks.delete")} aria-label={`${t("webhooks.delete")} ${hook.name}`} onClick={() => {
                            if (window.confirm(`${t("webhooks.delete_confirm")} ${hook.name}?`)) void run(() => deleteWebhook(hook.id));
                          }}>
                            <Trash2 size={15} />
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
