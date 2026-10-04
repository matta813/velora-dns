import { request } from "./api";

export interface Client {
  id: number;
  name: string;
  addresses: string[];
  group: string;
  description: string;
  enabled: boolean;
}

export interface ClientView extends Client {
  activity: { queries: number; last_seen: string | null } | null;
}

export interface ClientActivity {
  client_ip: string;
  queries: number;
  last_seen: string;
}

export interface ClientList {
  clients: ClientView[];
  unnamed: ClientActivity[];
}

export type ClientInput = Omit<Client, "id">;

export const loadClients = (signal?: AbortSignal) => request<ClientList>("/api/v1/clients", signal);
export const createClient = (input: ClientInput) =>
  request<Client>("/api/v1/clients", undefined, "POST", { body: input });
export const updateClient = (id: number, input: ClientInput) =>
  request<Client>(`/api/v1/clients/${id}`, undefined, "PUT", { body: input });
export const deleteClient = (id: number) =>
  request<{ deleted: number }>(`/api/v1/clients/${id}`, undefined, "DELETE");
