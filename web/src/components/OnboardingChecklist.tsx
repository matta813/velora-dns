import { CheckCircle2, Circle, X } from "lucide-react";
import { useEffect, useState } from "react";
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
  completed: boolean;
}

const DISMISS_KEY = "velora_onboarding_dismissed";

function readDismissed() {
  try {
    return typeof localStorage !== "undefined" && localStorage.getItem(DISMISS_KEY) === "true";
  } catch {
    return false;
  }
}

export function OnboardingChecklist() {
  const { t } = useI18n();
  const [status, setStatus] = useState<OnboardingStatus | null>(null);
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [dismissed, setDismissed] = useState(readDismissed);

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
        // Onboarding hints are optional; stay quiet when they cannot load.
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(poll, 30000);
      }
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, []);

  const handleDismiss = () => {
    try {
      localStorage.setItem(DISMISS_KEY, "true");
    } catch {
      // Dismissal still applies for this session.
    }
    setDismissed(true);
  };

  if (!status || !snapshot || !status.first_run || dismissed) {
    return null;
  }

  const items: ChecklistItem[] = [
    {
      id: "credentials",
      title: t("onboarding.change_credentials"),
      description: t("onboarding.change_credentials_text"),
      completed: status.user_count > 1,
    },
    {
      id: "dns_config",
      title: t("onboarding.review_dns"),
      description: t("onboarding.review_dns_text"),
      completed: snapshot.config.dns.upstreams.length > 0,
    },
    {
      id: "client_restriction",
      title: t("onboarding.restrict_clients"),
      description: t("onboarding.restrict_clients_text"),
      completed: snapshot.config.dns.allowed_clients.length > 0,
    },
    {
      id: "database",
      title: t("onboarding.verify_db"),
      description: t("onboarding.verify_db_text"),
      completed: true,
    },
    {
      id: "updates",
      title: t("onboarding.update_policy"),
      description: t("onboarding.update_policy_text"),
      completed: false,
    },
  ];

  const completedCount = items.filter((item) => item.completed).length;

  return (
    <section className="onboarding" aria-labelledby="onboarding-title">
      <div className="onboarding-head">
        <div>
          <h2 id="onboarding-title">{t("onboarding.welcome")}</h2>
          <p>
            {t("onboarding.complete_steps")} ({completedCount}/{items.length})
          </p>
        </div>
        <button className="button ghost small" onClick={handleDismiss}>
          <X size={14} />
          {t("onboarding.dismiss")}
        </button>
      </div>
      <div
        className="progress"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={items.length}
        aria-valuenow={completedCount}
        aria-labelledby="onboarding-title"
      >
        <span style={{ width: `${(completedCount / items.length) * 100}%` }} />
      </div>
      <ul className="checklist">
        {items.map((item) => (
          <li key={item.id} className={item.completed ? "done" : undefined}>
            {item.completed ? (
              <CheckCircle2 size={18} className="check-icon" />
            ) : (
              <Circle size={18} className="check-icon" />
            )}
            <div>
              <strong>{item.title}</strong>
              <p>{item.description}</p>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
