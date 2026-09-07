import { useState, type FormEvent } from "react";
import type { ZoneInput } from "./types";
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
      setError(e instanceof Error ? e.message : "Unable to create zone");
    }
  }
  return (
    <form
      className="zone-form"
      aria-label="Create DNS zone"
      onSubmit={(e) => void submit(e)}
    >
      <h3>Create a local zone</h3>
      <p>
        Velora will answer for this zone and keep unknown names inside your
        network.
      </p>
      <fieldset disabled={busy}>
        <label>
          Zone name
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
          <summary>Authority settings</summary>
          <div className="form-grid">
            <label>
              Primary nameserver
              <input
                value={primary}
                onChange={(e) => setPrimary(e.target.value)}
                placeholder="ns.home.arpa"
                maxLength={253}
              />
            </label>
            <label>
              Contact (DNS mailbox)
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
            {busy ? "Creating…" : "Create zone"}
          </button>
          <button className="button" type="button" onClick={cancel}>
            Cancel
          </button>
        </div>
      </fieldset>
    </form>
  );
}
