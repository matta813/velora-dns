# Encrypted backup and offline restore

Velora DNS can export a versioned encrypted backup of its effective configuration
and persistent SQLite database from **Backup & Restore** in the WebUI. This
includes zones, records, blocklists, users, settings, query history and other
SQLite state. The export is restricted to admins and uses the same session CSRF
and audit controls as other mutations. PostgreSQL backup/restore is not supported
by this workflow.

Choose a passphrase of at least 12 characters and store it separately from the
`.vdns` file. The bundle is encrypted with AES-256-CTR and authenticated with
HMAC-SHA-256; keys are derived with scrypt. Its contents include format version,
Velora version, timestamp, schema version, component list, `config.yaml`, and a
consistent SQLite snapshot. The snapshot uses SQLite `VACUUM INTO`, which
[produces a consistent live copy](https://www.sqlite.org/lang_vacuum.html).
The encrypted bundle is streamed to the browser and its server-side temporary
file is removed after the download. Treat both the passphrase and downloaded
bundle as sensitive.

## Inspect and restore

Restore is offline because replacing the database while the running resolver
holds in-memory zones and filters would leave the process inconsistent. Stop the
service first, then inspect and restore the bundle. Use the actual configuration
and database paths for your installation:

```bash
sudo systemctl stop velora-dns
read -rsp 'Backup passphrase: ' VELORA_BACKUP_PASSPHRASE; echo
export VELORA_BACKUP_PASSPHRASE
velora-dns -inspect-backup ./velora-backup.vdns
sudo -E velora-dns -restore-backup ./velora-backup.vdns \
  -config /etc/velora/config.yaml \
  -restore-database /var/lib/velora/velora.db
unset VELORA_BACKUP_PASSPHRASE
sudo systemctl start velora-dns
```

The inspect command authenticates the entire bundle, checks its schema and
configuration, then prints metadata without secrets. Restore repeats validation
before replacing files and refuses to proceed if a Velora process holds the
instance lock. A safety copy of the previous database and configuration is kept
in `velora-restore-safety-*` beside the database. If post-restore database, zone
or filter validation fails, the command restores the previous files. Keep the
safety directory until `/ready`, DNS resolution and your management workflow
have been checked. A process or machine crash during file replacement still
requires manual recovery from this safety copy.

The bundle must have a supported format and a database schema no newer than the
running binary. A wrong passphrase, tampered bundle, malformed archive, invalid
configuration or failed database integrity check is rejected before replacing
the installation. Keep enough free space for a SQLite snapshot, encrypted bundle,
restore staging and safety copy. The restored configuration retains its original
settings but uses the target `-restore-database` path.
