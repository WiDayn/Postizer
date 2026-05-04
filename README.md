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
- Title, slug, date, updated date, tags, summary, draft, and TOC fields.
- Markdown toolbar.
- Edit, split, and preview modes.
- Server-rendered live preview using the same Markdown pipeline as public posts.
- Save draft and publish actions that write to `content/posts/{slug}.md`.
- Media upload, recent media thumbnails, click-to-insert, and paste-to-upload.
- Media library upload, rename, caption/alt editing, figure copying, and delete actions.
- Optional home image upload in Settings for the front page image band.

## Frontend Method

Frontend work should start from the base component library in `web/static/site.css`, using `ui-*` primitives before adding page-specific classes. See `docs/FRONTEND.md` for the methodology and component inventory.
