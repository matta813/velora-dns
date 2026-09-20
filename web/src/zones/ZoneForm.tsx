import { useState, type FormEvent } from "react";
import type { ZoneInput } from "./types";
import { useI18n } from "../i18n-context";
export function ZoneForm({
  busy,
  save,
  cancel,
}: {
  busy: boolean;
  save: (input: ZoneInput) => Promise<void>;
  cancel: () => void;
}) {
  const [name, setName] = useState("");
  const [primary, setPrimary] = useState("");
  const [contact, setContact] = useState("");
  const { t } = useI18n();
  const [error, setError] = useState("");
  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    try {
      await save({
        name,
        primary_ns: primary || undefined,
        contact: contact || undefined,
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : t("zones.create_failed"));
    }
  }
  return (
    <form
      className="zone-form"
      aria-label={t("zones.create_zone_aria")}
      onSubmit={(e) => void submit(e)}
    >
      <h3>{t("zones.create_title")}</h3>
      <p>
        {t("zones.create_text")}
      </p>
      <fieldset disabled={busy}>
        <label>
          {t("zones.zone_name")}
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            autoFocus
            maxLength={253}
            placeholder="home.arpa"
            autoComplete="off"
          />
        </label>
        <details>
          <summary>{t("zones.authority_settings")}</summary>
          <div className="form-grid">
            <label>
              {t("zones.primary_nameserver")}
              <input
                value={primary}
                onChange={(e) => setPrimary(e.target.value)}
                placeholder="ns.home.arpa"
                maxLength={253}
              />
            </label>
            <label>
              {t("zones.contact_mailbox")}
              <input
                value={contact}
                onChange={(e) => setContact(e.target.value)}
                placeholder="hostmaster.home.arpa"
                maxLength={253}
              />
            </label>
          </div>
        </details>
        {error && (
          <p className="notice error" role="alert">
            {error}
          </p>
        )}
        <div className="form-actions">
          <button className="button primary" type="submit">
            {busy ? t("zones.creating") : t("zones.create_zone")}
          </button>
          <button className="button" type="button" onClick={cancel}>
            {t("zones.cancel")}
          </button>
        </div>
      </fieldset>
    </form>
  );
}
