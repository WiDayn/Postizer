# Postizer

Postizer is a Go-based, server-rendered blog system with Markdown, KaTeX math, pseudo-static URLs, and a black-and-white information-dense Times New Roman newspaper theme.

## Run

```powershell
go run ./cmd/postizer
```

Open:

- Public site: <http://localhost:8080/>
- Example post: <http://localhost:8080/posts/welcome>
- Example page: <http://localhost:8080/pages/about>
- Admin editor: <http://localhost:8080/admin>
- Media library: <http://localhost:8080/admin/media>

## Docker

Build and run with Docker Compose:

```bash
cp .env.example .env
# Edit .env and set a strong POSTIZER_ADMIN_PASSWORD and POSTIZER_SESSION_SECRET.
docker compose up -d --build
```

Open <http://localhost:8080/>. Compose stores runtime data in named volumes:

- `postizer-runtime`: the WordPress-style writable application runtime copied from the image on first start.
- `postizer-content`: posts, pages, settings, session secret, and admin credentials.
- `postizer-media`: uploaded media files and the media index.

The Docker image seeds `/app/runtime/current` from `/usr/src/postizer` when the runtime volume is empty, then starts Postizer from that writable runtime path. This lets future in-app updates replace the runtime files without depending on the Docker image tag.
When Admin -> Settings -> Auto Update is enabled, the Docker image checks `POSTIZER_REPO_URL` for the newest GitHub Release with a `vX.X.X` tag, downloads the matching `postizer-vX.X.X-linux-<arch>.tar.gz`, verifies `SHA256SUMS`, switches `/app/runtime/current`, and exits so Docker can restart into the new build.

For a direct Docker run:

```bash
docker build -t postizer:latest .
docker run -d --name postizer \
  -p 8080:8080 \
  -v postizer-runtime:/app/runtime \
  -v postizer-content:/app/content \
  -v postizer-media:/app/media \
  -e POSTIZER_ADMIN_USER=admin \
  -e POSTIZER_ADMIN_PASSWORD='change-this-password' \
  -e POSTIZER_SESSION_SECRET='change-this-long-random-secret' \
  -e POSTIZER_REPO_URL='https://github.com/WiDayn/Postizer.git' \
  -e POSTIZER_RELEASE_VERSION='latest' \
  postizer:latest
```

To publish the Docker image automatically, create these GitHub repository secrets:

- `DOCKERHUB_USERNAME`: your Docker Hub username or organization.
- `DOCKERHUB_TOKEN`: a Docker Hub access token with write access.

Then push a version tag:

```bash
git tag v0.1.2
git push origin v0.1.2
```

The release workflow publishes GitHub Release assets and pushes:

- `DOCKERHUB_USERNAME/postizer:v0.1.2`
- `DOCKERHUB_USERNAME/postizer:latest`

## Linux Service Install

On a Linux host with systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/WiDayn/Postizer/main/scripts/install-linux-service.sh | sudo bash -s -- \
  --service-name postizer \
  --port 8080 \
  --install-dir /opt/postizer \
  --env-file /etc/postizer/postizer.env \
  --bin-link /usr/local/bin/postizer \
  --update-service-name postizer-update \
  --update-timer-name postizer-update \
  --update-interval 15min
```

The installer downloads the latest GitHub Release asset matching the host architecture, verifies `SHA256SUMS`, installs the runtime into `/opt/postizer`, writes `/etc/postizer/postizer.env`, registers `postizer.service`, enables it, starts it, and registers `postizer-update.timer`. If `POSTIZER_ADMIN_PASSWORD` is not set, the installer generates an initial admin password and prints it once. Automatic updates are off by default and can be enabled in Admin -> Settings -> Auto Update; the timer only upgrades when a newer GitHub Release tag matching `vX.X.X` is available, so ordinary commits are ignored.

If you are already inside a cloned repository, you can also run:

```bash
bash scripts/install-linux-service.sh
```

Common options:

```bash
curl -fsSL https://raw.githubusercontent.com/WiDayn/Postizer/main/scripts/install-linux-service.sh | sudo env POSTIZER_ADMIN_PASSWORD='change-this-password' bash
curl -fsSL https://raw.githubusercontent.com/WiDayn/Postizer/main/scripts/install-linux-service.sh | sudo bash -s -- --addr 127.0.0.1:8080 --no-start
curl -fsSL https://raw.githubusercontent.com/WiDayn/Postizer/main/scripts/install-linux-service.sh | sudo bash -s -- --service-name postizer-blog2 --port 8081
curl -fsSL https://raw.githubusercontent.com/WiDayn/Postizer/main/scripts/install-linux-service.sh | sudo bash -s -- --release-version v1.2.3
bash scripts/install-linux-service.sh --binary ./postizer
bash scripts/install-linux-service.sh --skip-deps
bash scripts/install-linux-service.sh --build-from-source --source-dir /srv/postizer-src
bash scripts/install-linux-service.sh --no-update-timer
```

For multiple instances on one host, use a unique `--service-name` and port. Unless overridden, the installer derives isolated paths from the service name, such as `/opt/postizer-blog2`, `/etc/postizer-blog2/postizer-blog2.env`, `/usr/local/bin/postizer-blog2`, and `postizer-blog2-update.timer`. You can override the pieces individually with `--install-dir`, `--bin-link`, `--env-dir`, `--env-file`, `--update-service-name`, and `--update-timer-name`.

Installer options:

| Option | Description |
| --- | --- |
| `--install-dir PATH` | Runtime directory. Defaults to `/opt/<service-name>`. |
| `--release-version VERSION` | GitHub Release version to install. Defaults to `latest`. |
| `--repo-url URL` | GitHub repository used for release assets. Defaults to `https://github.com/WiDayn/Postizer.git`. |
| `--service-name NAME` | systemd service name. Defaults to `postizer`. |
| `--update-service-name NAME` | systemd oneshot update service name. Defaults to `<service-name>-update`. |
| `--update-timer-name NAME`, `--timer-name NAME` | systemd update timer name. Defaults to `<update-service-name>`. |
| `--update-interval N` | Auto-update timer interval. Defaults to `15min`. |
| `--addr ADDR` | `POSTIZER_ADDR` listen address, for example `127.0.0.1:8080`. |
| `--port PORT` | Listen on `:PORT`; shorthand for `--addr :PORT`. |
| `--user NAME` | Linux service user. Defaults to `postizer`. |
| `--group NAME` | Linux service group. Defaults to the service user. |
| `--bin-link PATH` | Symlink path for the installed binary. Defaults to `/usr/local/bin/<service-name>`. |
| `--env-dir PATH` | Environment file directory. Defaults to `/etc/<service-name>`. |
| `--env-file PATH` | Environment file path. Defaults to `<env-dir>/<service-name>.env`. |
| `--binary PATH` | Install an existing binary instead of building from source. |
| `--no-build` | Use `./postizer` from the source directory. |
| `--build-from-source` | Build from a Git checkout instead of using a release asset. |
| `--source-dir PATH` | Source checkout directory, only used with `--build-from-source`. |
| `--go-cache-dir PATH` | Go build/module cache root, only used with `--build-from-source`. |
| `--no-git-pull`, `--skip-git-pull` | Do not update an existing Git checkout before building from source. |
| `--skip-deps` | Do not install missing OS packages automatically. |
| `--no-update-timer` | Do not register the automatic update timer. |
| `--no-enable` | Do not enable the service at boot. |
| `--no-start` | Do not start or restart the service after installation. |

Service commands:

```bash
sudo systemctl status postizer
sudo journalctl -u postizer -f
sudo systemctl restart postizer
sudo systemctl status postizer-update.timer
```

Default local admin login:

- Username: `admin`
- Password: `postizer`

Override for real use:

```powershell
$env:POSTIZER_ADMIN_USER = "admin"
$env:POSTIZER_ADMIN_PASSWORD = "change-this-password"
$env:POSTIZER_SESSION_SECRET = "change-this-long-random-secret"
go run ./cmd/postizer
```

If `POSTIZER_SESSION_SECRET` is not set, Postizer creates `content/.session_secret` so local login sessions survive server restarts. The login form also has a "Remember me" option for a 30-day browser session.

## Content

- Posts: `content/posts/*.md`
- Pages: `content/pages/*.md`
- Optional tag metadata: `content/tags/*.md`

Posts and pages use YAML-like front matter followed by Markdown.

## Plugins

Postizer supports static resource plugins and gRPC process plugins. Static plugins can contain only translations, templates, or styles and do not need an executable. gRPC plugins declare a `runtime.kind` of `grpc` in their manifest; Postizer starts the plugin as a separate process and calls its `PluginService` over gRPC. Plugin admin UI is declared through manifest `ui_entries` pointing at static JSON files, so settings pages can render without starting the plugin process. Process plugins can import the public `pkg/pluginrpc` SDK and call the host `HostService` for generic operations such as job progress, media saves, post/page saves, and runtime reloads.

The WordPress importer is kept as an external example bundle under `examples/bundles/wordpress-importer`, not as part of the main application. Package that bundle and install it through `/admin/plugins`; installed user plugins live under `content/bundles`.

## Media

Images are uploaded into `media/public/{year}/{month}/`. The admin editor supports pasting an image from the clipboard and inserts a numbered figure at the current cursor position:

```markdown
\begin{figure}
![Image](/media/2026/05/hash.png)
\caption{Image}
\label{fig:post-1}
\end{figure}
```

Use `\figref{fig:post-1}` for a named figure reference.

`POSTIZER_ADMIN_TOKEN` can still be used for API automation with `Authorization: Bearer ...`, but browser admin access uses the login session.

## Editor

The admin editor at `/admin` includes:

- Login session authentication.
- Dedicated admin shell with sidebar pages.
- Dashboard, Posts, Editor, Media, and Settings sections.
- Post library with draft/published state.
- Title, automatic URL path with numeric duplicate suffixes, date, updated date, tags, summary, draft, and TOC fields.
- Markdown toolbar.
- Edit, split, and preview modes.
- Server-rendered live preview using the same Markdown pipeline as public posts.
- Save draft, publish, and delete actions for files in `content/posts/{title-slug}.md`.
- Media upload, recent media thumbnails, click-to-insert, and paste-to-upload.
- Media library upload, rename, caption/alt editing, figure copying, and delete actions.
- Optional home image upload in Settings for the front page image band.

## Frontend Method

Frontend work should start from the base component library in `web/static/site.css`, using `ui-*` primitives before adding page-specific classes. See `docs/FRONTEND.md` for the methodology and component inventory.
