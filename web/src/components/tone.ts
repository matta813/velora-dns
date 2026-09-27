export type BadgeTone = "neutral" | "success" | "warning" | "danger" | "info" | "brand";

/** Maps common backend health/status words onto a badge tone. */
export function toneFor(state: string | undefined): BadgeTone {
  switch (state) {
    case "healthy":
    case "completed":
    case "success":
    case "active":
    case "verified":
    case "valid":
      return "success";
    case "degraded":
    case "rolled_back":
    case "warning":
    case "stale":
      return "warning";
    case "failed":
    case "failure":
    case "unavailable":
    case "unreachable":
    case "critical":
    case "error":
      return "danger";
    case "info":
      return "info";
    default:
      return "neutral";
  }
}
