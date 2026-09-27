import { useCallback, useEffect, useState, type FormEvent } from "react";
import { FlaskConical, Pencil, Plus, RefreshCw, Route, Trash2 } from "lucide-react";
import {
  createForwardRule,
  deleteForwardRule,
  loadForwardRules,
  testForwardRule,
  updateForwardRule,
  type ForwardRuleInput,
  type ForwardRuleStatus,
  type ForwardTestResult,
} from "../api-forwarding";
import { EmptyState, Loading } from "../components/EmptyState";
import { toneFor } from "../components/tone";
import { useI18n } from "../i18n-context";
import "../zones/zones.css";

const TEST_TYPES = ["A", "AAAA", "CNAME", "MX", "NS", "PTR", "SOA", "TXT"];
const emptyForm = { domain: "", upstreams: "", description: "", enabled: true };

type Editor = { id: number | null; domain: string; upstreams: string; description: string; enabled: boolean };

export function Forwarding({ readOnly = false }: { readOnly?: boolean }) {
  const { t } = useI18n();
  const [rules, setRules] = useState<ForwardRuleStatus[] | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [editor, setEditor] = useState<Editor | null>(null);
  const [testing, setTesting] = useState<{ rule: ForwardRuleStatus; name: string; type: string } | null>(null);
  const [testResult, setTestResult] = useState<ForwardTestResult | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const next = await loadForwardRules(signal);
      if (!signal?.aborted) {
        setRules(next);
        setError("");
      }
    } catch (e) {
      if (!signal?.aborted) setError(e instanceof Error ? e.message : t("forwarding.load_failed"));
    }
  }, [t]);

  useEffect(() => {
    const controller = new AbortController();
    const initial = window.setTimeout(() => void load(controller.signal), 0);
    // Upstream health changes while queries flow; keep it reasonably fresh.
    const timer = window.setInterval(() => void load(controller.signal), 15000);
    return () => {
      controller.abort();
      window.clearTimeout(initial);
      window.clearInterval(timer);
    };
  }, [load]);

  const input = (form: Editor): ForwardRuleInput => ({
    domain: form.domain.trim(),
    upstreams: form.upstreams.split(/[\s,]+/).filter(Boolean),
    description: form.description.trim(),
    enabled: form.enabled,
  });

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!editor) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      if (editor.id === null) await createForwardRule(input(editor));
      else await updateForwardRule(editor.id, input(editor));
      setEditor(null);
      setNotice(t("forwarding.saved"));
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("forwarding.save_failed"));
    } finally {
      setBusy(false);
    }
  }

  async function toggle(rule: ForwardRuleStatus) {
    setBusy(true);
    setError("");
    try {
      await updateForwardRule(rule.id, { domain: rule.domain, upstreams: rule.upstreams, description: rule.description, enabled: !rule.enabled });
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("forwarding.save_failed"));
    } finally {
      setBusy(false);
    }
  }

  async function remove(rule: ForwardRuleStatus) {
    if (!window.confirm(`${t("forwarding.delete_confirm")} ${rule.domain}?`)) return;
    setBusy(true);
    setError("");
    try {
      await deleteForwardRule(rule.id);
      if (testing?.rule.id === rule.id) setTesting(null);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("forwarding.save_failed"));
    } finally {
      setBusy(false);
    }
  }

  async function runTest(event: FormEvent) {
    event.preventDefault();
    if (!testing) return;
    setBusy(true);
    setTestResult(null);
    setError("");
    try {
      setTestResult(await testForwardRule(testing.rule.id, testing.name.trim(), testing.type));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("forwarding.save_failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">{rules ? `${rules.length} ${t("forwarding.count")}` : ""}</span>
        <div>
          <button className="button" disabled={busy} onClick={() => void load()}>
            <RefreshCw size={15} />
            {t("blocklists.reload")}
          </button>
          <button
            className="button primary"
            disabled={busy || readOnly || editor !== null}
            onClick={() => {
              setEditor({ id: null, ...emptyForm });
              setNotice("");
            }}
          >
            <Plus size={15} />
            {t("forwarding.add")}
          </button>
        </div>
      </div>
      {error && <div className="notice error" role="alert">{error}</div>}
      {notice && <div className="notice success" role="status">{notice}</div>}
      {editor && (
        <section className="panel form-panel">
          <form
            className="zone-form"
            aria-label={editor.id === null ? t("forwarding.add_title") : t("forwarding.edit_title")}
            onSubmit={(event) => void save(event)}
          >
            <div>
              <h3>{editor.id === null ? t("forwarding.add_title") : t("forwarding.edit_title")}</h3>
              <p>{t("forwarding.form_hint")}</p>
            </div>
            <fieldset disabled={busy}>
              <div className="form-grid forward-grid">
                <label>
                  {t("forwarding.domain")}
                  <input
                    value={editor.domain}
                    onChange={(e) => setEditor({ ...editor, domain: e.target.value })}
                    placeholder="corp.example"
                    maxLength={253}
                    autoComplete="off"
                    required
                    autoFocus
                  />
                </label>
                <label>
                  {t("forwarding.upstreams")}
                  <input
                    value={editor.upstreams}
                    onChange={(e) => setEditor({ ...editor, upstreams: e.target.value })}
                    placeholder="10.0.0.10, 10.0.0.11:53"
                    autoComplete="off"
                    spellCheck={false}
                    required
                  />
                  <small>{t("forwarding.upstreams_hint")}</small>
                </label>
                <label>
                  {t("forwarding.description")}
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
                {t("forwarding.enabled")}
              </label>
              <div className="form-actions">
                <button className="button primary" type="submit">
                  {t("forwarding.save")}
                </button>
                <button className="button" type="button" onClick={() => setEditor(null)}>
                  {t("forwarding.cancel")}
                </button>
              </div>
            </fieldset>
          </form>
        </section>
      )}
      {testing && (
        <section className="panel form-panel">
          <form className="zone-form" aria-label={t("forwarding.test_title")} onSubmit={(event) => void runTest(event)}>
            <div>
              <h3>
                {t("forwarding.test_title")}: <code>{testing.rule.domain}</code>
              </h3>
              <p>{t("forwarding.test_hint")}</p>
            </div>
            <fieldset disabled={busy}>
              <div className="form-grid cache-filters">
                <label>
                  {t("forwarding.test_name")}
                  <input
                    value={testing.name}
                    onChange={(e) => setTesting({ ...testing, name: e.target.value })}
                    maxLength={253}
                    autoComplete="off"
                  />
                </label>
                <label>
                  {t("forwarding.test_type")}
                  <select value={testing.type} onChange={(e) => setTesting({ ...testing, type: e.target.value })}>
                    {TEST_TYPES.map((type) => (
                      <option key={type}>{type}</option>
                    ))}
                  </select>
                </label>
              </div>
              <div className="form-actions">
                <button className="button primary" type="submit">
                  <FlaskConical size={15} />
                  {t("forwarding.run_test")}
                </button>
                <button className="button" type="button" onClick={() => setTesting(null)}>
                  {t("forwarding.cancel")}
                </button>
              </div>
            </fieldset>
            {testResult && (
              <div className={`notice ${testResult.error ? "error" : "success"}`} role="status">
                <div>
                  <strong>
                    {testResult.name} {testResult.type}
                    {" → "}
                    {testResult.error ? testResult.error : testResult.rcode}
                  </strong>
                  {!testResult.error && (
                    <div>
                      {t("forwarding.test_via")} <code>{testResult.upstream}</code> · {testResult.duration_ms.toFixed(1)} ms
                    </div>
                  )}
                  {!testResult.error && (
                    testResult.answers.length
                      ? testResult.answers.map((answer, index) => <div key={index}><code>{answer}</code></div>)
                      : <div>{t("forwarding.no_answers")}</div>
                  )}
                </div>
              </div>
            )}
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
          <EmptyState icon={<Route size={22} />} title={t("forwarding.empty_title")}>
            {t("forwarding.empty_text")}
          </EmptyState>
        </section>
      )}
      {Boolean(rules?.length) && (
        <section className="panel">
          <div className="table-wrap">
            <table>
              <caption className="sr-only">{t("app.title.forwarding")}</caption>
              <thead>
                <tr>
                  <th>{t("forwarding.domain")}</th>
                  <th>{t("forwarding.upstreams")}</th>
                  <th>{t("forwarding.col_status")}</th>
                  <th>{t("forwarding.description")}</th>
                  <th>
                    <span className="sr-only">{t("zones.col_actions")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {rules?.map((rule) => (
                  <tr key={rule.id}>
                    <td>
                      <span className="endpoint">
                        <button
                          className="switch"
                          role="switch"
                          aria-checked={rule.enabled}
                          aria-label={`${t("forwarding.toggle")} ${rule.domain}`}
                          disabled={busy || readOnly}
                          onClick={() => void toggle(rule)}
                        />
                        <code>{rule.domain}</code>
                      </span>
                    </td>
                    <td className="wrap">
                      {rule.upstreams.map((upstream) => (
                        <div key={upstream}>
                          <code>{upstream}</code>
                        </div>
                      ))}
                    </td>
                    <td className="wrap">
                      {!rule.enabled ? (
                        <span className="status-text neutral">{t("forwarding.disabled")}</span>
                      ) : (
                        rule.health.map((health) => (
                          <div key={health.address}>
                            <span className={`status-text ${toneFor(health.state)}`}>
                              {t(`dashboard.upstream_${health.state}`)}
                            </span>
                            {health.state === "healthy" && (
                              <span className="latency">{Math.round(health.latency_milliseconds)} ms</span>
                            )}
                          </div>
                        ))
                      )}
                    </td>
                    <td className="wrap cell-muted">{rule.description || "—"}</td>
                    <td>
                      <div className="table-actions">
                        <button
                          className="icon-button"
                          disabled={busy || readOnly}
                          title={t("forwarding.test")}
                          aria-label={`${t("forwarding.test")} ${rule.domain}`}
                          onClick={() => {
                            setTesting({ rule, name: rule.domain, type: "A" });
                            setTestResult(null);
                          }}
                        >
                          <FlaskConical size={15} />
                        </button>
                        <button
                          className="icon-button"
                          disabled={busy || readOnly || editor !== null}
                          title={t("forwarding.edit")}
                          aria-label={`${t("forwarding.edit")} ${rule.domain}`}
                          onClick={() => {
                            setEditor({ id: rule.id, domain: rule.domain, upstreams: rule.upstreams.join(", "), description: rule.description, enabled: rule.enabled });
                            setNotice("");
                          }}
                        >
                          <Pencil size={15} />
                        </button>
                        <button
                          className="icon-button danger-icon"
                          disabled={busy || readOnly}
                          title={t("forwarding.delete")}
                          aria-label={`${t("forwarding.delete")} ${rule.domain}`}
                          onClick={() => void remove(rule)}
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
