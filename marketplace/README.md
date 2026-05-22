# Postizer Resource Marketplace

The marketplace is a static index. Postizer reads this JSON file, shows the
listed resource packs in the admin UI, and installs the ZIP asset from the pack
repository's GitHub Release.

Entries belong in `packs/index.json`:

```json
{
  "id": "editorial-tools",
  "name": "Editorial Tools",
  "summary": "A bundle with a writing theme and an export plugin.",
  "description": "Installs one theme and one plugin from a single release asset.",
  "repo": "https://github.com/example/postizer-pack-editorial-tools",
  "preview": "/marketplace/previews/editorial-tools.svg",
  "tags": ["theme", "plugin"],
  "themes": [
    {
      "id": "minimal-paper",
      "name": "Minimal Paper",
      "version": "1.0.0"
    }
  ],
  "plugins": [
    {
      "id": "content-exporter",
      "name": "Content Exporter",
      "version": "1.0.0"
    }
  ],
  "release": {
    "tag": "v1.0.0",
    "asset": "editorial-tools-v1.0.0.zip",
    "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  },
  "min_postizer": "v0.1.4"
}
```

Release assets must be `.zip` bundle packs with a top-level `manifest.json`.
The top-level manifest must use the same `id` as the index entry and its
`source_url` must point to the indexed GitHub repository.

Use the `theme` and `plugin` tags to describe what the bundle contains. The
admin UI currently exposes those two tags as the built-in filters.

Ready-to-publish standalone repository skeletons live under `repositories/`.
For example, `repositories/postizer-content-exporter` can be pushed to
`widayn/postizer-content-exporter`; its release workflow builds the plugin
binaries and attaches the marketplace ZIP asset plus checksums.
