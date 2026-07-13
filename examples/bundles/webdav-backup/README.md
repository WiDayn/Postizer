# WebDAV Backup

This bundle backs up all Postizer posts, pages, and media to a WebDAV server.
It supports manual backups, interval-based background backups, Basic Auth,
self-signed certificates (opt-in), and retention cleanup.

Build an installable multi-platform resource pack from the repository root:

```bash
bash scripts/build-webdav-backup-pack.sh
```

Install `dist/plugins/webdav-backup-v1.0.0.zip` from **Admin → Appearance →
Upload resource pack**, enable **WebDAV Backup**, then configure it under
**Admin → Plugins**. For Nextcloud, use an app password and a server URL such
as `https://cloud.example.com/remote.php/dav/files/USERNAME`.

The plugin stores credentials in `content/plugin-data/webdav-backup/config.json`
with owner-only file permissions. Backups use Postizer's content-export format
and include `content/posts`, `content/pages`, and `media`.
