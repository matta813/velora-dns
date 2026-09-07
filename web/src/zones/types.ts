export const recordTypes = [
  "A",
  "AAAA",
  "CNAME",
  "TXT",
  "MX",
  "NS",
  "PTR",
] as const;
export type RecordType = (typeof recordTypes)[number];
export interface ZoneRecord {
  id: number;
  name: string;
  type: RecordType;
  ttl: number;
  value: string;
  priority: number;
}
export interface Zone {
  id: number;
  name: string;
  primary_ns: string;
  contact: string;
  revision: number;
  records: ZoneRecord[];
}
export type RecordInput = Omit<ZoneRecord, "id">;
export interface ZoneInput {
  name: string;
  primary_ns?: string;
  contact?: string;
}
export function ownerName(name: string, zone: string) {
  return name === zone
    ? "@"
    : name.endsWith(`.${zone}`)
      ? name.slice(0, -zone.length - 1)
      : name;
}
