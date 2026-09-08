#!/bin/sh
set -eu
docker compose config --quiet
docker compose -f docker-compose.yml -f configs/compose.lan.example.yaml config --quiet
python3 - <<'CHECK'
from pathlib import Path
import re
text=Path('docker-compose.yml').read_text()
for setting in ('read_only: true','cap_drop: [ALL]','no-new-privileges:true','127.0.0.1:${VELORA_DNS_PORT','127.0.0.1:${VELORA_HTTP_PORT'):
    assert setting in text,setting
for line in Path('Dockerfile').read_text().splitlines():
    if line.startswith('FROM '):
        assert re.search(r'@sha256:[a-f0-9]{64}',line),line
assert 'USER 10001:10001' in Path('Dockerfile').read_text()
lan=Path('configs/compose.lan.example.yaml').read_text()
for setting in ("published: '53'", 'protocol: udp', 'protocol: tcp', 'host_ip: 127.0.0.1', 'fd12:3456:789a::/64'):
    assert setting in lan,setting
print('Compose hardening and image pins verified')
CHECK
