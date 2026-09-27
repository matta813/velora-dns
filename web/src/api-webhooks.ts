import { request } from "./api";

export type Severity = "info" | "warning" | "critical";

export interface Webhook {
  id: number;
  name: string;
  url: string;
  events: string[];
  min_severity: Severity;
  allow_private: boolean;
  enabled: boolean;
  has_token: boolean;
  last_delivery_at: string | null;
  last_status: string;
  last_error: string;
  consecutive_failures: number;
}

export interface WebhookInput {
  name: string;
  url: string;
  events: string[];
  min_severity: Severity;
  allow_private: boolean;
  enabled: boolean;
  /** Omit to keep the stored token, "" to remove it. */
  token?: string;
}

export interface WebhookTestResult {
  ok: boolean;
  error: string;
  webhook: Webhook;
}

export const loadWebhooks = (signal?: AbortSignal) => request<Webhook[]>("/api/v1/webhooks", signal);
export const loadEventTypes = (signal?: AbortSignal) => request<string[]>("/api/v1/webhooks/event-types", signal);
export const createWebhook = (input: WebhookInput) =>
  request<Webhook>("/api/v1/webhooks", undefined, "POST", { body: input });
export const updateWebhook = (id: number, input: WebhookInput) =>
  request<Webhook>(`/api/v1/webhooks/${id}`, undefined, "PUT", { body: input });
export const deleteWebhook = (id: number) =>
  request<{ deleted: number }>(`/api/v1/webhooks/${id}`, undefined, "DELETE");
export const testWebhook = (id: number) =>
  request<WebhookTestResult>(`/api/v1/webhooks/${id}/test`, undefined, "POST");
