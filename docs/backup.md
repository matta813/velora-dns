# Encrypted backup and restore

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

## Restore from the web interface

Admins can restore a bundle from **Backup & Restore → Restore a backup**:

1. **Check backup.** The bundle and passphrase are uploaded (up to 2 GiB) and
   fully validated: authentication, format, schema compatibility, database
   integrity, configuration validity, and that zones and filters load. The page
   then shows the backup's date and Velora version, the schema change, what it
   contains (zones, records, blocklists, rewrites, forwarding rules, clients,
   users) and warnings such as changed listen addresses or a backup without
   users. **Nothing is changed at this point.** The decrypted, validated files
   are kept in a private staging directory next to the database for 30 minutes.
2. **Restore and restart.** After you confirm, the staged files become the
   pending restore and Velora exits with status 75 so its supervisor starts it
   again (`Restart=on-failure` in the systemd unit, `restart: unless-stopped` in
   Docker Compose). If Velora runs without a supervisor, start it again yourself.
3. **Apply.** On start, before the database is opened, the pending restore
   replaces the database and configuration. The previous files are copied to a
   `velora-restore-safety-*` directory first, and any failure while swapping or
   loading the restored data puts them back.
4. **Confirm.** When the restarted server is ready (DNS and HTTP listening) the
   restore is marked **completed**. If a start with the restored data exits
   before that — for example because a restored listen address cannot be bound
   — the next start restores the safety copy automatically and reports
   **rolled back** with the reason.

The page follows the restart and shows the result. Sessions come from the
restored database, so you usually have to sign in again with an account from
the backup. The last result stays visible on the page (`GET
/api/v1/backup/restore`). Inspecting and restoring are admin-only, require CSRF
tokens for session requests and are audited; the passphrase is only sent with
the upload and never stored.

Online restore supports SQLite installations. PostgreSQL installations should
use PostgreSQL's own backup tools.

## Restore from the command line

The command line restore works offline. Stop the service first, then inspect and
restore the bundle. Use the actual configuration and database paths for your
installation:

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
