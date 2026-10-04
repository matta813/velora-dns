import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { Pencil, Plus, RefreshCw, Replace, Search, Trash2 } from "lucide-react";
import {
  createRewrite,
  deleteRewrite,
  loadRewrites,
  updateRewrite,
  type RewriteInput,
  type RewriteStatus,
  type RewriteType,
} from "../api-rewrites";
import { Badge } from "../components/Badge";
import { EmptyState, Loading } from "../components/EmptyState";
import { useI18n } from "../i18n-context";
import "../zones/zones.css";

const TYPES: RewriteType[] = ["A", "AAAA", "CNAME"];
const PLACEHOLDERS: Record<RewriteType, string> = { A: "192.168.1.20", AAAA: "2001:db8::20", CNAME: "nas.home" };

type Editor = RewriteInput & { id: number | null };

export function Rewrites({ readOnly = false }: { readOnly?: boolean }) {
  const { t } = useI18n();
  const [rules, setRules] = useState<RewriteStatus[] | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [editor, setEditor] = useState<Editor | null>(null);
  const [params] = useSearchParams();
  const [filter, setFilter] = useState(() => params.get("q") ?? "");

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const next = await loadRewrites(signal);
      if (!signal?.aborted) {
        setRules(next);
        setError("");
      }
    } catch (e) {
      if (!signal?.aborted) setError(e instanceof Error ? e.message : t("rewrites.load_failed"));
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

  const visible = useMemo(() => {
    const needle = filter.trim().toLowerCase();
    if (!rules || !needle) return rules;
    return rules.filter((rule) => rule.name.includes(needle) || rule.value.toLowerCase().includes(needle));
  }, [rules, filter]);

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
      setError(e instanceof Error ? e.message : t("rewrites.save_failed"));
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!editor) return;
    const { id, ...input } = editor;
    const body = { ...input, name: input.name.trim(), value: input.value.trim(), description: input.description.trim() };
    if (await run(() => (id === null ? createRewrite(body) : updateRewrite(id, body)), t("rewrites.saved"))) setEditor(null);
  }

  const fromRule = (rule: RewriteStatus): RewriteInput => ({
    name: rule.name, type: rule.type, value: rule.value, enabled: rule.enabled, description: rule.description,
  });

  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">{rules ? `${rules.length} ${t("rewrites.count")}` : ""}</span>
        <div>
          <button className="button" disabled={busy} onClick={() => void load()}>
            <RefreshCw size={15} />
            {t("blocklists.reload")}
          </button>
          <button
            className="button primary"
            disabled={busy || readOnly || editor !== null}
            onClick={() => {
              setEditor({ id: null, name: "", type: "A", value: "", enabled: true, description: "" });
              setNotice("");
            }}
          >
            <Plus size={15} />
            {t("rewrites.add")}
          </button>
        </div>
      </div>
      {error && <div className="notice error" role="alert">{error}</div>}
      {notice && <div className="notice success" role="status">{notice}</div>}
      {editor && (
        <section className="panel form-panel">
          <form
            className="zone-form"
            aria-label={editor.id === null ? t("rewrites.add_title") : t("rewrites.edit_title")}
            onSubmit={(event) => void save(event)}
          >
            <div>
              <h3>{editor.id === null ? t("rewrites.add_title") : t("rewrites.edit_title")}</h3>
              <p>{t("rewrites.form_hint")}</p>
            </div>
            <fieldset disabled={busy}>
              <div className="form-grid rewrite-grid">
                <label>
                  {t("rewrites.name")}
                  <input
                    value={editor.name}
                    onChange={(e) => setEditor({ ...editor, name: e.target.value })}
                    placeholder="nas.home"
                    maxLength={255}
                    autoComplete="off"
                    required
                    autoFocus
                  />
                </label>
                <label>
                  {t("rewrites.type")}
                  <select value={editor.type} onChange={(e) => setEditor({ ...editor, type: e.target.value as RewriteType })}>
                    {TYPES.map((type) => (
                      <option key={type}>{type}</option>
                    ))}
                  </select>
                </label>
                <label>
                  {t("rewrites.value")}
                  <input
                    value={editor.value}
                    onChange={(e) => setEditor({ ...editor, value: e.target.value })}
                    placeholder={PLACEHOLDERS[editor.type]}
                    maxLength={253}
                    autoComplete="off"
                    spellCheck={false}
                    required
                  />
                  <small>{t(`rewrites.value_hint_${editor.type}`)}</small>
                </label>
                <label>
                  {t("rewrites.description")}
                  <input
                    value={editor.description}
                    onChange={(e) => setEditor({ ...editor, description: e.target.value })}
                    maxLength={200}
                  />
                </label>
              </div>
              <label className="check-field">
                <input
                  type="checkbox"
                  checked={editor.enabled}
                  onChange={(e) => setEditor({ ...editor, enabled: e.target.checked })}
                />
                {t("rewrites.enabled")}
              </label>
              <div className="form-actions">
                <button className="button primary" type="submit">
                  {t("rewrites.save")}
                </button>
                <button className="button" type="button" onClick={() => setEditor(null)}>
                  {t("forwarding.cancel")}
                </button>
              </div>
            </fieldset>
          </form>
        </section>
      )}
      {rules === null && !error && (
        <section className="panel">
          <Loading>{t("app.connecting_panel")}</Loading>
        </section>
      )}
      {rules?.length === 0 && (
        <section className="panel">
          <EmptyState icon={<Replace size={22} />} title={t("rewrites.empty_title")}>
            {t("rewrites.empty_text")}
          </EmptyState>
        </section>
      )}
      {Boolean(rules?.length) && (
        <section className="panel">
          <div className="zone-form" role="search">
            <label className="zone-search rewrite-search">
              <Search size={15} />
              <span className="sr-only">{t("rewrites.search")}</span>
              <input
                type="search"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                placeholder={t("rewrites.search")}
                autoComplete="off"
              />
            </label>
          </div>
          <div className="table-wrap">
            <table className="records-table">
              <caption className="sr-only">{t("app.title.rewrites")}</caption>
              <thead>
                <tr>
                  <th>{t("rewrites.name")}</th>
                  <th>{t("rewrites.type")}</th>
                  <th>{t("rewrites.value")}</th>
                  <th>{t("rewrites.col_notes")}</th>
                  <th>
                    <span className="sr-only">{t("zones.col_actions")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {visible?.map((rule) => (
                  <tr key={rule.id}>
                    <td>
                      <span className="endpoint">
                        <button
                          className="switch"
                          role="switch"
                          aria-checked={rule.enabled}
                          aria-label={`${t("rewrites.toggle")} ${rule.name} ${rule.type}`}
                          disabled={busy || readOnly}
                          onClick={() => void run(() => updateRewrite(rule.id, { ...fromRule(rule), enabled: !rule.enabled }))}
                        />
                        <code>{rule.name}</code>
                      </span>
                    </td>
                    <td><span className="record-type">{rule.type}</span></td>
                    <td className="record-value"><code>{rule.value}</code></td>
                    <td className="wrap">
                      {rule.blocked_by && <Badge tone="danger">{t("rewrites.blocked_hint")}</Badge>}
                      {rule.overrides_zone && (
                        <Badge tone="info">{t("rewrites.overrides_hint")}: {rule.overrides_zone}</Badge>
                      )}
                      {rule.description && <small className="cell-muted">{rule.description}</small>}
                    </td>
                    <td>
                      <div className="table-actions">
                        <button
                          className="icon-button"
                          disabled={busy || readOnly || editor !== null}
                          title={t("rewrites.edit")}
                          aria-label={`${t("rewrites.edit")} ${rule.name} ${rule.type}`}
                          onClick={() => {
                            setEditor({ id: rule.id, ...fromRule(rule) });
                            setNotice("");
                          }}
                        >
                          <Pencil size={15} />
                        </button>
                        <button
                          className="icon-button danger-icon"
                          disabled={busy || readOnly}
                          title={t("rewrites.delete")}
                          aria-label={`${t("rewrites.delete")} ${rule.name} ${rule.type}`}
                          onClick={() => {
                            if (window.confirm(`${t("rewrites.delete_confirm")}\n${rule.name} ${rule.type} ${rule.value}`)) {
                              void run(() => deleteRewrite(rule.id));
                            }
                          }}
                        >
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
    </>
  );
}
