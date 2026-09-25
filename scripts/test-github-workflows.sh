#!/bin/sh
set -eu
python3 - <<'CHECK'
from pathlib import Path
import re
for file in Path('.github/workflows').glob('*.yml'):
    text=file.read_text()
    for action in re.findall(r'uses: ([^\s]+)', text):
        assert re.search(r'@[a-f0-9]{40}$',action), (file,action)
release=Path('.github/workflows/release.yml').read_text()
assert "if: vars.RELEASE_ENABLED == 'true'" in release
assert 'paths: [RELEASE]' in release
assert 'ref: ${{ needs.validate.outputs.source_sha }}' in release
assert 'provenance: mode=max' in release
assert 'contents: read' in release
for name in ('release.yml', 'release-beta.yml'):
    workflow = Path('.github/workflows', name).read_text()
    assert 'cache-mode: none' in workflow, name
    assert 'cache: npm' not in workflow, name
    assert 'cache: false' in workflow, name
ci=Path('.github/workflows/ci.yml').read_text()
assert 'push: true' not in ci
for ecosystem in ('gomod','npm','docker','github-actions'):
    assert f'package-ecosystem: {ecosystem}' in Path('.github/dependabot.yml').read_text()
print('Workflow invariants passed')
CHECK
