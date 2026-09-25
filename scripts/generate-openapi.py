#!/usr/bin/env python3
"""Generate and validate the management OpenAPI contract against Go routes."""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "docs/openapi.json"
ROUTE = re.compile(r'mux\.Handle(?:Func)?\("(GET|POST|PUT|DELETE|PATCH) (/api/v1/[^\"]+)"')


def ref(name):
    return {"$ref": f"#/components/schemas/{name}"}


def obj(properties, required=()):
    result = {"type": "object", "properties": properties}
    if required:
        result["required"] = list(required)
    return result


def arr(item):
    return {"type": "array", "items": item}


STRING = {"type": "string"}
INT = {"type": "integer", "format": "int64"}
NUMBER = {"type": "number"}
BOOL = {"type": "boolean"}
TIME = {"type": "string", "format": "date-time"}
STRINGS = arr(STRING)

SCHEMAS = {
    "Error": obj({"code": STRING, "message": STRING}, ("code", "message")),
    "ErrorResponse": obj({"error": ref("Error")}, ("error",)),
    "Version": obj({"version": STRING, "commit": STRING, "built": STRING}, ("version", "commit", "built")),
    "Status": obj({"ready": BOOL, "uptime_seconds": NUMBER, "dns_listen": STRINGS, "version": ref("Version"), "capabilities": STRINGS}, ("ready", "uptime_seconds", "dns_listen", "version", "capabilities")),
    "Statistics": obj({"queries_total": INT, "blocked_queries": INT, "queries_per_second": NUMBER, "cache_hit_rate": NUMBER}, ("queries_total", "blocked_queries", "queries_per_second", "cache_hit_rate")),
    "CacheStats": obj({"entries": INT, "capacity": INT, "hits": INT, "misses": INT}, ("entries", "capacity", "hits", "misses")),
    "RateLimitSettings": obj({"enabled": BOOL, "global_qps": INT, "client_qps": INT, "rate_limit_burst": INT}, ("enabled", "global_qps", "client_qps", "rate_limit_burst")),
    "RateLimitStatus": obj({"enabled": BOOL, "rejected_total": INT, "last_rejected_at": TIME}, ("enabled", "rejected_total")),
    "UpstreamStatus": obj({"address": STRING, "state": {"type": "string", "enum": ["unknown", "healthy", "degraded", "unavailable"]}, "consecutive_failures": INT, "latency_milliseconds": NUMBER, "last_success": TIME, "last_failure": TIME}, ("address", "state", "consecutive_failures", "latency_milliseconds")),
    "DNSConfig": obj({"listen": STRINGS, "upstreams": STRINGS, "allowed_clients": STRINGS, "timeout": INT, "retries": INT, "max_concurrent": INT, "rate_limit_enabled": BOOL, "global_qps": INT, "client_qps": INT, "rate_limit_burst": INT, "max_tcp_connections": INT, "dot_listen": STRING, "doh_listen": STRING, "doq_listen": STRING, "dnssec": BOOL, "trust_anchors": STRINGS}),
    "Config": obj({"dns": ref("DNSConfig"), "cache": obj({"max_entries": INT, "upstream_ttl": INT}), "http": obj({"listen": STRING, "web_dir": STRING, "allowed_hosts": STRINGS}), "filtering": obj({"block_mode": STRING, "blocklist": STRINGS, "allowlist": STRINGS}), "query_log": obj({"enabled": BOOL, "retention": INT, "max_rows": INT, "queue_size": INT}), "log_level": STRING}),
    "ZoneRecord": obj({"id": INT, "name": STRING, "type": STRING, "ttl": INT, "value": STRING, "priority": INT}, ("name", "type", "ttl", "value")),
    "Zone": obj({"id": INT, "name": STRING, "zone_type": STRING, "revision": INT, "primary_ns": STRING, "contact": STRING, "records": arr(ref("ZoneRecord")), "primary_address": STRING, "transfer_tsig_key": STRING, "transfer_interval": INT, "last_transfer_at": TIME, "next_refresh_at": TIME, "last_transfer_serial": INT}, ("id", "name", "revision", "records")),
    "ZoneInput": obj({"name": STRING, "primary_ns": STRING, "contact": STRING, "records": arr(ref("ZoneRecord"))}, ("name",)),
    "SecondaryZoneInput": obj({"name": STRING, "primary_address": STRING, "transfer_tsig_key": STRING, "transfer_interval": INT}, ("name", "primary_address")),
    "SecondaryZone": obj({"id": INT, "name": STRING, "zone_type": STRING, "primary_address": STRING, "transfer_tsig_key": STRING, "transfer_interval": INT, "last_transfer_serial": INT, "last_transfer_at": TIME, "next_refresh_at": TIME}, ("id", "name", "zone_type", "primary_address")),
    "Blocklist": obj({"id": INT, "name": STRING, "url": STRING, "enabled": BOOL, "domain_count": INT, "last_updated_at": TIME, "last_error": STRING}, ("id", "name", "url", "enabled")),
    "BlocklistInput": obj({"name": STRING, "url": STRING, "enabled": BOOL}, ("name", "url")),
    "Query": obj({"id": INT, "occurred_at": TIME, "client_ip": STRING, "domain": STRING, "type": STRING, "rcode": STRING, "duration": NUMBER, "source": STRING, "upstream": STRING, "cache_hit": BOOL}, ("id", "domain", "type", "rcode", "source")),
    "QueryRanking": obj({"value": STRING, "count": INT}, ("value", "count")),
    "QuerySummary": obj({"window_start": TIME, "window_end": TIME, "total": INT, "blocked": INT, "top_domains": arr(ref("QueryRanking")), "top_clients": arr(ref("QueryRanking"))}),
    "AuditEvent": obj({"id": INT, "occurred_at": TIME, "actor": STRING, "role": STRING, "action": STRING, "target": STRING, "result": STRING, "status_code": INT}),
    "SystemEvent": obj({"id": INT, "severity": {"type": "string", "enum": ["info", "warning", "critical"]}, "title": STRING, "message": STRING, "link": STRING, "occurred_at": TIME, "repeat_count": INT, "read": BOOL}, ("id", "severity", "title", "message", "occurred_at", "repeat_count", "read")),
    "SystemEventPage": obj({"events": arr(ref("SystemEvent")), "unread_count": INT}, ("events", "unread_count")),
    "EventReadResult": obj({"read": BOOL}, ("read",)),
    "BackupStatus": obj({"supported": BOOL, "last_backup_time": TIME, "last_backup_size": INT, "backup_age": STRING, "verification_state": STRING, "database_path": STRING}, ("supported", "verification_state", "database_path")),
    "BackupVerification": obj({"valid": BOOL, "schema_version": INT, "record_count": INT, "error": STRING}, ("valid", "schema_version", "record_count")),
    "User": obj({"id": INT, "username": STRING, "role": {"type": "string", "enum": ["admin", "operator", "viewer"]}, "disabled": BOOL}, ("id", "username", "role")),
    "Session": obj({"username": STRING, "role": STRING, "csrf_token": STRING, "language": STRING, "theme": STRING}, ("username", "role", "csrf_token", "language", "theme")),
    "LogoutResult": obj({"logged_out": BOOL}, ("logged_out",)),
    "Preferences": obj({"language": STRING, "theme": STRING}),
    "TSIGKey": obj({"name": STRING, "algorithm": STRING}, ("name", "algorithm")),
    "TSIGKeyInput": obj({"name": STRING, "algorithm": STRING, "secret": {"type": "string", "format": "password", "writeOnly": True}}, ("name", "algorithm", "secret")),
    "TokenInput": obj({"name": STRING, "scopes": arr({"type": "string", "enum": ["read", "write", "admin"]}), "expires_in_hours": INT}, ("name", "scopes", "expires_in_hours")),
    "TokenCreated": obj({"id": INT, "name": STRING, "scopes": STRINGS, "expires_at": TIME, "token": {"type": "string", "writeOnly": True}}, ("id", "name", "scopes", "expires_at", "token")),
    "DHCPPool": obj({"id": INT, "name": STRING, "network": STRING, "start_ip": STRING, "end_ip": STRING}),
    "DHCPLease": obj({"id": INT, "ip_address": STRING, "mac_address": STRING, "hostname": STRING, "expires_at": TIME}),
    "DHCPReservation": obj({"id": INT, "ip_address": STRING, "mac_address": STRING, "hostname": STRING}),
    "Node": obj({"id": STRING, "name": STRING, "address": STRING, "capabilities": STRINGS, "version": STRING, "status": STRING, "last_seen_at": TIME}),
    "ConfigVersion": obj({"version": INT, "config_hash": STRING, "config": ref("Config"), "applied_by": STRING, "applied_at": TIME}),
    "CacheEntry": obj({"name": STRING, "type": STRING, "rcode": STRING, "answers": STRINGS, "remaining_ttl": INT}),
    "CacheEntryPage": obj({"entries": arr(ref("CacheEntry")), "total": INT}),
    "OperationResult": obj({"status": STRING, "message": STRING, "deleted": INT, "revoked": INT}),
    "OperationalReport": obj({"generated_at": TIME, "version": ref("Version"), "os": STRING, "architecture": STRING, "uptime_seconds": INT, "state": STRING, "components": arr(obj({"name": STRING, "state": STRING, "detail": STRING})), "upstream_count": INT, "query_log_enabled": BOOL}),
    "UpdateState": obj({"state": STRING, "installed": STRING, "from_version": STRING, "to_version": STRING, "channel": STRING, "started_at": TIME, "last_completed": TIME, "updating": BOOL, "error": STRING, "rollback_used": BOOL, "readiness_ok": BOOL}),
    "UpdateHealth": obj({"ready": BOOL}, ("ready",)),
    "UpdateCheck": obj({"installed_version": STRING, "latest_version": STRING, "update_available": BOOL, "channel": STRING, "release_date": TIME, "release_notes": STRING, "architecture": STRING, "download_size": INT}),
    "UpdateRequestResult": obj({"status": STRING, "version": STRING, "message": STRING}),
    "UpdateHistoryEntry": obj({"id": STRING, "started_at": TIME, "completed_at": TIME, "from_version": STRING, "to_version": STRING, "channel": STRING, "state": STRING, "error": STRING, "readiness_ok": BOOL, "rollback_used": BOOL, "deployment_mode": STRING}),
    "OnboardingStatus": obj({"first_run": BOOL, "user_count": INT, "config_ready": BOOL}),
    "FlexibleObject": obj({"status": STRING, "message": STRING}),
}

RESPONSE_MODELS = {
    "/api/v1/status": "Status", "/api/v1/version": "Version", "/api/v1/stats": "Statistics", "/api/v1/stats/reset": "Statistics",
    "/api/v1/cache": "CacheStats", "/api/v1/cache/entries": "CacheEntryPage", "/api/v1/config": "Config",
    "/api/v1/preferences": "Preferences", "/api/v1/settings/rate-limit": "RateLimitSettings", "/api/v1/settings/rate-limit/status": "RateLimitStatus",
    "/api/v1/upstreams/health": ["UpstreamStatus"], "/api/v1/zones": ["Zone"], "/api/v1/zones/{id}": "Zone",
    "/api/v1/zones/secondary": "SecondaryZone", "/api/v1/zones/{id}/transfer-status": "SecondaryZone",
    "/api/v1/zones/{id}/records": ["ZoneRecord"], "/api/v1/zones/{id}/records/{recordID}": "ZoneRecord",
    "/api/v1/blocklists": ["Blocklist"], "/api/v1/blocklists/{id}": "Blocklist", "/api/v1/blocklists/{id}/content": "Blocklist", "/api/v1/blocklists/{id}/update": "Blocklist",
    "/api/v1/queries": ["Query"], "/api/v1/query-stats": "QuerySummary", "/api/v1/audit": ["AuditEvent"],
    "/api/v1/events": "SystemEventPage", "/api/v1/events/{id}/read": "EventReadResult",
    "/api/v1/backup/status": "BackupStatus", "/api/v1/backup/verify": "BackupVerification", "/api/v1/users": ["User"],
    "/api/v1/auth/login": "Session", "/api/v1/auth/me": "Session", "/api/v1/auth/logout": "LogoutResult", "/api/v1/preferences": "Preferences",
    "/api/v1/tokens": "TokenCreated", "/api/v1/tsig-keys": ["TSIGKey"], "/api/v1/onboarding/status": "OnboardingStatus",
    "/api/v1/diagnostics": "OperationalReport", "/api/v1/dhcp/pools": ["DHCPPool"], "/api/v1/dhcp/leases": ["DHCPLease"],
    "/api/v1/dhcp/pools/{id}/reservations": ["DHCPReservation"],
    "/api/v1/dhcp/reservations/{id}": "DHCPReservation", "/api/v1/cluster/nodes": ["Node"],
    "/api/v1/cluster/config-versions": ["ConfigVersion"],
    "/api/v1/update/health": "UpdateHealth", "/api/v1/update/status": "UpdateState", "/api/v1/update/check": "UpdateCheck", "/api/v1/update/request": "UpdateRequestResult", "/api/v1/update/history": ["UpdateHistoryEntry"],
    "/api/v1/users/{id}/password": "OperationResult", "/api/v1/users/{id}/role": "OperationResult", "/api/v1/tokens/{id}": "OperationResult",
}
REQUEST_MODELS = {
    ("POST", "/api/v1/auth/login"): obj({"username": STRING, "password": {"type": "string", "format": "password", "writeOnly": True}}, ("username", "password")),
    ("PUT", "/api/v1/config"): ref("Config"),
    ("PUT", "/api/v1/preferences"): ref("Preferences"),
    ("PUT", "/api/v1/settings/rate-limit"): ref("RateLimitSettings"),
    ("POST", "/api/v1/zones"): ref("ZoneInput"),
    ("PUT", "/api/v1/zones/{id}"): ref("ZoneInput"),
    ("POST", "/api/v1/zones/secondary"): ref("SecondaryZoneInput"),
    ("POST", "/api/v1/zones/{id}/records"): ref("ZoneRecord"),
    ("PUT", "/api/v1/zones/{id}/records/{recordID}"): ref("ZoneRecord"),
    ("POST", "/api/v1/blocklists"): ref("BlocklistInput"),
    ("PUT", "/api/v1/blocklists/{id}"): obj({"enabled": BOOL}, ("enabled",)),
    ("PUT", "/api/v1/blocklists/{id}/content"): obj({"content": STRING}, ("content",)),
    ("POST", "/api/v1/backup/verify"): obj({"path": STRING}, ("path",)),
    ("POST", "/api/v1/backup/create"): obj({"passphrase": {"type": "string", "format": "password", "writeOnly": True, "minLength": 12}}, ("passphrase",)),
    ("POST", "/api/v1/update/request"): obj({"action": {"type": "string", "enum": ["update"]}}, ("action",)),
    ("POST", "/api/v1/users"): obj({"username": STRING, "password": {"type": "string", "format": "password", "writeOnly": True}, "role": STRING}, ("username", "password", "role")),
    ("POST", "/api/v1/tokens"): ref("TokenInput"),
    ("POST", "/api/v1/tsig-keys"): ref("TSIGKeyInput"),
    ("PUT", "/api/v1/users/{id}/role"): obj({"role": STRING}, ("role",)),
    ("PUT", "/api/v1/users/{id}/password"): obj({"password": {"type": "string", "format": "password", "writeOnly": True}}, ("password",)),
}


def response_model(method, path):
    if path == "/api/v1/backup/create":
        return {"application/octet-stream": {"schema": {"type": "string", "format": "binary"}}}
    if path.endswith("/export"):
        return {"text/dns": {"schema": STRING}}
    if path in RESPONSE_MODELS:
        model = RESPONSE_MODELS[path]
        if method == "POST" and isinstance(model, list):
            model = model[0]
    elif "/zones/" in path:
        model = "Zone" if "records" not in path else "ZoneRecord"
    elif path == "/api/v1/zones/import":
        model = "Zone"
    elif path.startswith("/api/v1/blocklists/"):
        model = "Blocklist"
    elif path.startswith("/api/v1/dhcp/"):
        model = "DHCPPool" if "pools" in path else "DHCPLease"
    elif path.startswith("/api/v1/cluster/nodes/"):
        model = "Node"
    elif path.startswith("/api/v1/cluster/config-versions/"):
        model = "ConfigVersion"
    elif path.startswith("/api/v1/users/"):
        model = "User"
    else:
        model = "OperationResult" if method == "DELETE" else "FlexibleObject"
    schema = arr(ref(model[0])) if isinstance(model, list) else ref(model)
    return {"application/json": {"schema": obj({"data": schema}, ("data",))}}


def operation(method, path):
    ident = re.sub(r"[^A-Za-z0-9]+", "_", path.removeprefix("/api/v1/")).strip("_")
    op = {
        "operationId": method.lower() + "_" + ident,
        "summary": method.title() + " " + path.removeprefix("/api/v1/"),
        "tags": [path.split("/")[3] if len(path.split("/")) > 3 else "management"],
        "responses": {
            "202" if method == "POST" and path == "/api/v1/update/request" else "201" if method == "POST" and path in {"/api/v1/users", "/api/v1/tokens", "/api/v1/tsig-keys", "/api/v1/zones", "/api/v1/zones/secondary", "/api/v1/zones/import", "/api/v1/zones/{id}/records", "/api/v1/blocklists", "/api/v1/dhcp/pools", "/api/v1/dhcp/pools/{id}/reservations"} else "200": {"description": "Successful response", "content": response_model(method, path)},
            "default": {"description": "Error; codes include authentication_required, insufficient_role, invalid_csrf, invalid_json, and endpoint-specific validation/storage codes.", "content": {"application/json": {"schema": ref("ErrorResponse")}}},
        },
    }
    if path == "/api/v1/auth/login":
        op["security"] = []
    elif path == "/api/v1/auth/logout":
        op["security"] = [{"sessionCookie": [], "csrfHeader": []}]
    elif method in ("POST", "PUT", "DELETE", "PATCH"):
        op["security"] = [{"bearerAuth": []}, {"sessionCookie": [], "csrfHeader": []}]
    else:
        op["security"] = [{"bearerAuth": []}, {"sessionCookie": []}]
    if path == "/api/v1/auth/login":
        op["x-required-role"] = "anonymous"
    elif path.startswith("/api/v1/users") or path == "/api/v1/audit" or path == "/api/v1/stats/reset" or path == "/api/v1/backup/create":
        op["x-required-role"] = "admin"
    elif method in ("POST", "PUT", "DELETE", "PATCH"):
        op["x-required-role"] = "operator-or-admin"
    else:
        op["x-required-role"] = "viewer-or-higher"
    if path.startswith("/api/v1/events"):
        op["x-required-role"] = "viewer-or-higher"
    if path.endswith("/read") and path.startswith("/api/v1/events/"):
        op["description"] = "A viewer may acknowledge an event using a session and CSRF token. Bearer tokens require write or admin scope."
    if "{" in path:
        op["parameters"] = [{"name": name, "in": "path", "required": True, "schema": STRING if name == "name" or path.startswith("/api/v1/cluster/nodes/") else INT} for name in re.findall(r"\{([^}]+)\}", path)]
    if method in ("POST", "PUT", "PATCH"):
        schema = REQUEST_MODELS.get((method, path))
        if schema is None and path.startswith("/api/v1/dhcp/"):
            schema = ref("DHCPReservation" if "reservations" in path else "DHCPPool")
        if schema is not None:
            op["requestBody"] = {"required": True, "content": {"application/json": {"schema": schema}}}
    if method in ("PUT", "DELETE") and path.startswith("/api/v1/zones/{id}"):
        op.setdefault("parameters", []).append({"name": "If-Match", "in": "header", "required": True, "description": "Quoted zone revision (ETag)", "schema": STRING})
    if path == "/api/v1/zones/import":
        op["requestBody"] = {"required": True, "content": {"text/dns": {"schema": STRING}}}
    if method == "PUT" and path == "/api/v1/config":
        op["responses"]["409"] = {"description": "config_requires_restart; response message names fields without a live apply path", "content": {"application/json": {"schema": ref("ErrorResponse")}}}
    if method == "DELETE" and path.startswith("/api/v1/cluster/"):
        op["summary"] = "Cluster node removal is unavailable until a membership protocol exists"
        op["responses"] = {"501": {"description": "Cluster membership changes are unavailable"}, "default": op["responses"]["default"]}
    return op


def generate():
    routes = set()
    for source in (ROOT / "internal/api").glob("*.go"):
        if source.name.endswith("_test.go"):
            continue
        routes.update(ROUTE.findall(source.read_text()))
    paths = {}
    for method, path in sorted(routes, key=lambda item: (item[1], item[0])):
        paths.setdefault(path, {})[method.lower()] = operation(method, path)
    return {
        "openapi": "3.1.0",
        "info": {"title": "Velora DNS Management API", "version": "1.0.0", "description": "Session cookies require X-CSRF-Token on mutations. API bearer tokens bypass CSRF and require the documented scope. All API responses use a data or error envelope except text/dns zone export. Secrets are never returned by read endpoints."},
        "servers": [{"url": "http://127.0.0.1:8080"}],
        "paths": paths,
        "components": {"securitySchemes": {
            "bearerAuth": {"type": "http", "scheme": "bearer", "description": "velora_ API token; scopes read, write, or admin"},
            "sessionCookie": {"type": "apiKey", "in": "cookie", "name": "velora_session"},
            "csrfHeader": {"type": "apiKey", "in": "header", "name": "X-CSRF-Token", "description": "Required for cookie-authenticated mutations"},
        }, "schemas": SCHEMAS},
    }


def validate(spec):
    names = set(spec["components"]["schemas"])
    ids = set()
    def visit(value):
        if isinstance(value, dict):
            if "$ref" in value and value["$ref"].removeprefix("#/components/schemas/") not in names:
                raise ValueError(f"unknown schema reference: {value['$ref']}")
            for child in value.values():
                visit(child)
        elif isinstance(value, list):
            for child in value:
                visit(child)
    visit(spec)
    for path, methods in spec["paths"].items():
        for method, op in methods.items():
            if op["operationId"] in ids:
                raise ValueError(f"duplicate operationId: {op['operationId']}")
            ids.add(op["operationId"])
            if not op["responses"] or "security" not in op:
                raise ValueError(f"incomplete operation: {method} {path}")
    if not spec["openapi"].startswith("3.") or not spec["paths"]:
        raise ValueError("not an OpenAPI 3 document")


spec = generate()
validate(spec)
content = json.dumps(spec, indent=2, ensure_ascii=False) + "\n"
if "--check" in sys.argv:
    if not OUTPUT.exists() or OUTPUT.read_text() != content:
        sys.exit("OpenAPI document is stale; run python3 scripts/generate-openapi.py")
else:
    OUTPUT.write_text(content)
