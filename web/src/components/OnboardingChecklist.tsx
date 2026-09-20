import { CheckCircle, Circle, ShieldCheck, Users, Globe, Database, Download } from "lucide-react";
import { useEffect, useState, useCallback } from "react";
import { loadSnapshot, type Snapshot } from "../api";
import { useI18n } from "../i18n-context";

interface OnboardingStatus {
  first_run: boolean;
  user_count: number;
  config_ready: boolean;
}

interface ChecklistItem {
  id: string;
  title: string;
  description: string;
  icon: React.ReactNode;
  completed: boolean;
}

export function OnboardingChecklist() {
  const { t } = useI18n();
  const [status, setStatus] = useState<OnboardingStatus | null>(null);
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [dismissed, setDismissed] = useState(
    () => typeof localStorage !== "undefined" && localStorage.getItem("velora_onboarding_dismissed") === "true",
  );
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(() => setLoading((v) => v), []);

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const [s, snap] = await Promise.all([
          fetch("/api/v1/onboarding/status").then((r) => r.json()).then((d: { data: OnboardingStatus }) => d.data),
          loadSnapshot(controller.signal),
        ]);
        if (controller.signal.aborted) return;
        setStatus(s);
        setSnapshot(snap);
      } catch {
        // Silently handle errors for onboarding
      } finally {
        setLoading(false);
        if (!controller.signal.aborted) timer = setTimeout(poll, 30000);
      }
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [refresh]);

  const handleDismiss = () => {
    if (typeof localStorage !== "undefined") {
      localStorage.setItem("velora_onboarding_dismissed", "true");
    }
    setDismissed(true);
  };

  if (loading || !status || !snapshot) {
    return null;
  }

  if (!status.first_run || dismissed) {
    return null;
  }

  const items: ChecklistItem[] = [
    {
      id: "credentials",
      title: t("onboarding.change_credentials"),
      description: t("onboarding.change_credentials_text"),
      icon: <Users size={18} />,
      completed: status.user_count > 1,
    },
    {
      id: "dns_config",
      title: t("onboarding.review_dns"),
      description: t("onboarding.review_dns_text"),
      icon: <Globe size={18} />,
      completed: snapshot.config.dns.upstreams.length > 0,
    },
    {
      id: "client_restriction",
      title: t("onboarding.restrict_clients"),
      description: t("onboarding.restrict_clients_text"),
      icon: <ShieldCheck size={18} />,
      completed: snapshot.config.dns.allowed_clients.length > 0,
    },
    {
      id: "database",
      title: t("onboarding.verify_db"),
      description: t("onboarding.verify_db_text"),
      icon: <Database size={18} />,
      completed: true,
    },
    {
      id: "updates",
      title: t("onboarding.update_policy"),
      description: t("onboarding.update_policy_text"),
      icon: <Download size={18} />,
      completed: false,
    },
  ];

  const completedCount = items.filter((item) => item.completed).length;

  return (
    <div className="panel padded" style={{ marginBottom: "1rem", borderLeft: "3px solid var(--accent, #3b82f6)" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: "1rem" }}>
        <div>
          <h2 style={{ margin: 0 }}>{t("onboarding.welcome")}</h2>
          <p style={{ margin: "0.25rem 0 0", opacity: 0.7 }}>
            {t("onboarding.complete_steps")} ({completedCount}/{items.length})
          </p>
        </div>
        <button className="button secondary" onClick={handleDismiss}>
{t("onboarding.dismiss")}
        </button>
      </div>
      <div style={{ display: "grid", gap: "0.75rem" }}>
        {items.map((item) => (
          <div
            key={item.id}
            style={{
              display: "flex",
              alignItems: "flex-start",
              gap: "0.75rem",
              padding: "0.75rem",
              borderRadius: "6px",
              backgroundColor: item.completed ? "rgba(34, 197, 94, 0.05)" : "rgba(59, 130, 246, 0.05)",
            }}
          >
            <div style={{ marginTop: "2px" }}>
              {item.completed ? (
                <CheckCircle size={18} style={{ color: "var(--success, #22c55e)" }} />
              ) : (
                <Circle size={18} style={{ opacity: 0.4 }} />
              )}
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                {item.icon}
                <strong>{item.title}</strong>
              </div>
              <p style={{ margin: "0.25rem 0 0", fontSize: "0.85rem", opacity: 0.7 }}>
                {item.description}
              </p>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
