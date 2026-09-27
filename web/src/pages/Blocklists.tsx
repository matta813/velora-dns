import { useEffect, useState } from "react";
import { Plus, RefreshCw, ShieldBan, Trash2, Pencil } from "lucide-react";
import { request, type BlocklistSource } from "../api";
import { useI18n } from "../i18n-context";
import { Badge } from "../components/Badge";
import { EmptyState, Loading } from "../components/EmptyState";

const SCHEDULES: { seconds: number; key: string }[] = [
  { seconds: 0, key: "blocklists.manual" },
  { seconds: 3600, key: "blocklists.every_1h" },
  { seconds: 6 * 3600, key: "blocklists.every_6h" },
  { seconds: 12 * 3600, key: "blocklists.every_12h" },
  { seconds: 24 * 3600, key: "blocklists.every_24h" },
  { seconds: 7 * 24 * 3600, key: "blocklists.every_7d" },
];

export function Blocklists({ readOnly = false }: { readOnly?: boolean }) {
  const { t, language } = useI18n();
  const [sources, setSources] = useState<BlocklistSource[] | null>(null);
  const [name, setName] = useState("");
  const [url, setURL] = useState("");
  const [schedule, setSchedule] = useState(24 * 3600);
  const [adding, setAdding] = useState(false);
  const [busy, setBusy] = useState<number | "new" | null>(null);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<number | null>(null);
  const [editContent, setEditContent] = useState("");
  const [confirmDelete, setConfirmDelete] = useState<number | null>(null);

  const load = async () => {
    setError("");
    try {
      setSources(await request<BlocklistSource[]>("/api/v1/blocklists"));
    } catch (e) {
      setError(e instanceof Error ? e.message : t("blocklists.load_failed"));
    }
  };
  useEffect(() => {
    let active = true;
    void request<BlocklistSource[]>("/api/v1/blocklists")
      .then((data) => {
        if (active) {
          setSources(data);
          setError("");
        }
      })
      .catch((e) => {
        if (active) {
          setError(e instanceof Error ? e.message : t("blocklists.load_failed"));
        }
      });
    return () => {
      active = false;
    };
  }, [t]);
  const add = async () => {
    setBusy("new");
    setError("");
    try {
      await request("/api/v1/blocklists", undefined, "POST", {
        body: { name, url, update_interval: url.trim() ? schedule : 0 },
      });
      setName("");
      setURL("");
      setAdding(false);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("blocklists.add_failed"));
    } finally {
      setBusy(null);
    }
  };
  const toggleEnabled = async (id: number, currentEnabled: boolean) => {
    setBusy(id);
    setError("");
    try {
      await request(`/api/v1/blocklists/${id}`, undefined, "PUT", {
        body: { enabled: !currentEnabled },
      });
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("blocklists.toggle_failed"));
    } finally {
      setBusy(null);
    }
  };
  const changeSchedule = async (id: number, seconds: number) => {
    setBusy(id);
    setError("");
    try {
      await request(`/api/v1/blocklists/${id}`, undefined, "PUT", {
        body: { update_interval: seconds },
      });
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("blocklists.schedule_failed"));
    } finally {
      setBusy(null);
    }
  };
  const refresh = async (id: number) => {
    setBusy(id);
    setError("");
    try {
      await request(`/api/v1/blocklists/${id}/update`, undefined, "POST");
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("blocklists.update_failed_msg"));
    } finally {
      setBusy(null);
    }
  };
  const deleteSource = async (id: number) => {
    setBusy(id);
    setError("");
    try {
      await request(`/api/v1/blocklists/${id}`, undefined, "DELETE");
      setConfirmDelete(null);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("blocklists.delete_failed"));
    } finally {
      setBusy(null);
    }
  };
  const saveContent = async (id: number) => {
    setBusy(id);
    setError("");
    try {
      await request(`/api/v1/blocklists/${id}/content`, undefined, "PUT", {
        body: { content: editContent },
      });
      setEditing(null);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : t("blocklists.content_save_failed"));
    } finally {
      setBusy(null);
    }
  };
  const startEditing = async (id: number) => {
    setError("");
    try {
      const source = await request<BlocklistSource>(`/api/v1/blocklists/${id}`);
      setEditContent(
        (source.domains ?? []).join("\n")
      );
      setEditing(id);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("blocklists.load_failed"));
    }
  };
  return (
    <>
      <div className="zones-toolbar">
        <span className="zone-count">
          {sources
            ? `${sources.length} ${sources.length === 1 ? t("blocklists.count_one") : t("blocklists.count")}`
            : t("blocklists.sources_label")}
        </span>
        <div>
          <button className="button" onClick={() => void load()} disabled={busy !== null}>
            <RefreshCw size={15} />
            {t("blocklists.reload")}
          </button>
          <button
            className="button primary"
            onClick={() => setAdding(true)}
            disabled={busy !== null || readOnly}
          >
            <Plus size={15} />
            {t("blocklists.add_source")}
          </button>
        </div>
      </div>
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}
      {adding && (
        <section className="panel form-panel">
          <form
            className="zone-form"
            aria-label={t("blocklists.add_aria")}
            onSubmit={(e) => {
              e.preventDefault();
              void add();
            }}
          >
            <div>
              <h3>{t("blocklists.add_title")}</h3>
              <p>{t("blocklists.add_text")}</p>
            </div>
            <fieldset disabled={busy !== null}>
              <div className="form-grid source-grid">
                <label>
                  {t("blocklists.name")}
                  <input
                    required
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder={t("blocklists.community_hosts")}
                  />
                </label>
                <label>
                  {t("blocklists.source_url")}
                  <input
                    required
                    type="url"
                    value={url}
                    onChange={(e) => setURL(e.target.value)}
                    placeholder="https://example.org/hosts.txt"
                  />
                </label>
                <label>
                  {t("blocklists.schedule")}
                  <select value={schedule} onChange={(e) => setSchedule(Number(e.target.value))}>
                    {SCHEDULES.map((option) => (
                      <option key={option.seconds} value={option.seconds}>
                        {t(option.key)}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <div className="form-actions">
                <button className="button primary">
                  {busy === "new" ? t("blocklists.adding") : t("blocklists.add_source")}
                </button>
                <button type="button" className="button" onClick={() => setAdding(false)}>
                  {t("blocklists.cancel")}
                </button>
              </div>
            </fieldset>
          </form>
        </section>
      )}
      {editing !== null && (
        <section className="panel form-panel">
          <form
            className="zone-form"
            aria-label={t("blocklists.edit_content_aria")}
            onSubmit={(e) => {
              e.preventDefault();
              void saveContent(editing);
            }}
          >
            <div>
              <h3>{t("blocklists.edit_content_title")}</h3>
              <p>{t("blocklists.edit_content_text")}</p>
            </div>
            <fieldset disabled={busy !== null}>
              <label>
                {t("blocklists.domains_label")}
                <textarea
                  value={editContent}
                  onChange={(e) => setEditContent(e.target.value)}
                  rows={12}
                  placeholder={"0.0.0.0 ads.example.com\ntracker.example.com"}
                />
              </label>
              <div className="form-actions">
                <button className="button primary">
                  {busy === editing ? t("blocklists.saving") : t("blocklists.save")}
                </button>
                <button type="button" className="button" onClick={() => setEditing(null)}>
                  {t("blocklists.cancel")}
                </button>
              </div>
            </fieldset>
          </form>
        </section>
      )}
      {sources === null ? (
        <section className="panel">
          <Loading>{t("blocklists.loading")}</Loading>
        </section>
      ) : sources.length === 0 ? (
        <section className="panel">
          <EmptyState icon={<ShieldBan size={22} />} title={t("blocklists.empty_title")}>
            {t("blocklists.empty_text")}
          </EmptyState>
        </section>
      ) : (
        <section className="panel">
          <div className="table-wrap">
            <table className="records-table">
              <caption className="sr-only">{t("blocklists.caption")}</caption>
              <thead>
                <tr>
                  <th>{t("blocklists.name")}</th>
                  <th>{t("blocklists.col_source")}</th>
                  <th className="num">{t("blocklists.col_domains")}</th>
                  <th>{t("blocklists.col_status")}</th>
                  <th>{t("blocklists.schedule")}</th>
                  <th>{t("blocklists.col_last_update")}</th>
                  <th>{t("blocklists.col_next_update")}</th>
                  <th>
                    <span className="sr-only">{t("zones.col_actions")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {sources.map((source) => (
                  <tr key={source.id}>
                    <td>
                      <span className="endpoint">
                        <button
                          className="switch"
                          role="switch"
                          aria-checked={source.enabled}
                          aria-label={`${source.enabled ? t("blocklists.disable") : t("blocklists.enable")}: ${source.name}`}
                          title={source.enabled ? t("blocklists.disable") : t("blocklists.enable")}
                          disabled={busy !== null || readOnly}
                          onClick={() => void toggleEnabled(source.id, source.enabled)}
                        />
                        <strong>{source.name}</strong>
                      </span>
                    </td>
                    <td className="record-value">
                      {source.url ? <code>{source.url}</code> : <Badge tone="neutral" plain>{t("blocklists.local")}</Badge>}
                    </td>
                    <td className="num mono">
                      {(source.domain_count ?? source.domains?.length ?? 0).toLocaleString(language)}
                    </td>
                    <td>
                      {source.last_error ? (
                        <span title={source.last_error}>
                          <Badge tone="danger">{t("blocklists.update_failed")}</Badge>
                        </span>
                      ) : source.url && !source.last_updated_at ? (
                        <Badge tone="warning">{t("blocklists.never_updated")}</Badge>
                      ) : source.enabled ? (
                        <Badge tone="success">{t("blocklists.active")}</Badge>
                      ) : (
                        <Badge>{t("blocklists.disabled")}</Badge>
                      )}
                    </td>
                    <td>
                      {source.url ? (
                        <select
                          className="inline-select"
                          aria-label={`${t("blocklists.schedule_for")} ${source.name}`}
                          value={source.update_interval ?? 0}
                          disabled={busy !== null || readOnly}
                          onChange={(e) => void changeSchedule(source.id, Number(e.target.value))}
                        >
                          {SCHEDULES.map((option) => (
                            <option key={option.seconds} value={option.seconds}>
                              {t(option.key)}
                            </option>
                          ))}
                        </select>
                      ) : (
                        <span className="cell-muted">—</span>
                      )}
                    </td>
                    <td className="cell-muted">
                      {source.last_updated_at
                        ? new Date(source.last_updated_at).toLocaleString()
                        : t("blocklists.never")}
                      {(source.consecutive_failures ?? 0) > 0 && (
                        <small className="danger">
                          {source.consecutive_failures} {t("blocklists.retry_count")}
                        </small>
                      )}
                    </td>
                    <td className="cell-muted">
                      {source.next_update_at
                        ? new Date(source.next_update_at) <= new Date()
                          ? t("blocklists.due_now")
                          : new Date(source.next_update_at).toLocaleString()
                        : "—"}
                    </td>
                    <td>
                      <div className="blocklist-actions">
                        {confirmDelete === source.id ? (
                          <>
                            <button
                              className="button danger small"
                              disabled={busy !== null}
                              onClick={() => void deleteSource(source.id)}
                            >
                              {t("blocklists.confirm_delete")}
                            </button>
                            <button className="button ghost small" onClick={() => setConfirmDelete(null)}>
                              {t("blocklists.cancel")}
                            </button>
                          </>
                        ) : (
                          <>
                            {!source.url && (
                              <button
                                className="icon-button"
                                title={t("blocklists.edit_content")}
                                aria-label={t("blocklists.edit_content")}
                                disabled={busy !== null || readOnly}
                                onClick={() => void startEditing(source.id)}
                              >
                                <Pencil size={15} />
                              </button>
                            )}
                            <button
                              className="icon-button"
                              title={t("blocklists.update")}
                              aria-label={busy === source.id ? t("blocklists.updating") : t("blocklists.update")}
                              disabled={busy !== null || readOnly}
                              onClick={() => void refresh(source.id)}
                            >
                              <RefreshCw size={15} className={busy === source.id ? "spin" : undefined} />
                            </button>
                            <button
                              className="icon-button danger-icon"
                              title={t("blocklists.delete")}
                              aria-label={t("blocklists.delete")}
                              disabled={busy !== null || readOnly}
                              onClick={() => setConfirmDelete(source.id)}
                            >
                              <Trash2 size={15} />
                            </button>
                          </>
                        )}
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
