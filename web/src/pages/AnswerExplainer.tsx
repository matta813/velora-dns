import { useState, type FormEvent } from "react";
import { Search } from "lucide-react";
import { EXPLAIN_TYPES, explainAnswer, type AnswerExplanation, type ExplainRule, type ExplainStep } from "../api-explain";
import { Badge } from "../components/Badge";
import { useI18n } from "../i18n-context";
import "../zones/zones.css";

const tones: Record<string, "success" | "danger" | "info" | "neutral"> = {
  answered: "success",
  hit: "success",
  allowed: "success",
  blocked: "danger",
  error: "danger",
  would_forward: "info",
};

const ruleLabel = (rule: ExplainRule) => (rule.id ? `${rule.name} (#${rule.id})` : rule.name) + (rule.detail ? ` · ${rule.detail}` : "");

function StepDetails({ step }: { step: ExplainStep }) {
  const { t } = useI18n();
  const rules = step.rules ?? [];
  const filter = step.filter;
  return (
    <>
      {rules.map((rule) => (
        <div key={ruleLabel(rule)} className="cell-muted">{ruleLabel(rule)}</div>
      ))}
      {filter && (
        <div className="cell-muted">
          {filter.scope === "client_policy"
            ? `${t("explain.scope.client_policy")}: ${filter.client ?? ""} (${filter.mode ?? ""})`
            : t("explain.scope.global")}
          {(filter.matches ?? []).map((rule) => ` · ${t("explain.matches")}: ${ruleLabel(rule)}`)}
          {(filter.allowed_by ?? []).map((rule) => ` · ${t("explain.allowed_by")}: ${ruleLabel(rule)}`)}
        </div>
      )}
      {step.remaining_ttl !== undefined && <div className="cell-muted">{t("explain.ttl")}: {step.remaining_ttl} s</div>}
      {step.result === "would_forward" && <div className="cell-muted">{t("explain.not_contacted")}</div>}
    </>
  );
}

export function AnswerExplainer() {
  const { t } = useI18n();
  const [name, setName] = useState("");
  const [type, setType] = useState("A");
  const [client, setClient] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<AnswerExplanation | null>(null);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      setResult(await explainAnswer({ name: name.trim(), type, client: client.trim() || undefined }));
    } catch (reason) {
      setResult(null);
      setError(reason instanceof Error ? reason.message : t("explain.failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <h2>{t("explain.title")}</h2>
          <p>{t("explain.hint")}</p>
        </div>
      </div>
      <form className="zone-form" aria-label={t("explain.title")} onSubmit={(event) => void submit(event)}>
        <fieldset disabled={busy}>
          <div className="form-grid forward-grid">
            <label>
              {t("explain.name")}
              <input value={name} onChange={(e) => setName(e.target.value)} placeholder="nas.home" maxLength={253} autoComplete="off" spellCheck={false} required />
            </label>
            <label>
              {t("explain.type")}
              <select value={type} onChange={(e) => setType(e.target.value)}>
                {EXPLAIN_TYPES.map((value) => <option key={value}>{value}</option>)}
              </select>
            </label>
            <label>
              {t("explain.client")}
              <input value={client} onChange={(e) => setClient(e.target.value)} placeholder="192.168.1.40" maxLength={45} autoComplete="off" spellCheck={false} />
              <small>{t("explain.client_hint")}</small>
            </label>
          </div>
          <button className="button primary" type="submit"><Search size={15} />{busy ? t("explain.running") : t("explain.submit")}</button>
        </fieldset>
      </form>
      {error && <div className="notice error" role="alert">{error}</div>}
      {result && (
        <div className="panel-body" role="region" aria-label={t("explain.result")}>
          <p role="status" className="diagnostic-summary">
            <Badge tone={tones[result.winner.result] ?? "neutral"}>{t(`explain.source.${result.source}`)}</Badge>
            <strong>{t("explain.winner")}: {t(`explain.stage.${result.winner.stage}`)}</strong>
          </p>
          <p className="cell-muted">
            {result.name} {result.type}{result.client ? ` · ${result.client}` : ""}{result.rcode ? ` · ${result.rcode}` : ""}
          </p>
          {result.answers.length > 0 ? (
            <ul aria-label={t("explain.answers")}>{result.answers.map((answer) => <li key={answer}><code>{answer}</code></li>)}</ul>
          ) : (
            <p className="cell-muted">{t("explain.no_answers")}</p>
          )}
          <div className="table-wrap">
            <table>
              <caption className="sr-only">{t("explain.steps")}</caption>
              <thead><tr><th>{t("explain.step")}</th><th>{t("explain.outcome")}</th><th>{t("explain.detail")}</th></tr></thead>
              <tbody>{result.steps.map((step, index) => (
                <tr key={index}>
                  <td><strong>{step.depth > 0 ? `↳ ${t("explain.cname_target")} · ` : ""}{t(`explain.stage.${step.stage}`)}</strong></td>
                  <td><Badge tone={tones[step.result] ?? "neutral"}>{t(`explain.result.${step.result}`)}</Badge></td>
                  <td className="wrap"><StepDetails step={step} /></td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  );
}
