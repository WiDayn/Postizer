# Postizer Design

## Product Shape

Postizer is a Go-based personal/blog publishing system for long-form technical writing. The reading experience favors dense information, fast navigation, mathematical notation, code blocks, footnotes, pages, tags, archives, and a restrained black-and-white newspaper editorial style.

## Core Goals

- Serve Markdown posts and pages with math formula support.
- Make response speed the first architectural priority.
- Keep pages fast, readable, and indexable.
- Support information-dense layouts: table of contents, metadata, backlinks, tags, archive lists, and compact navigation.
- Use Go for rendering, routing, content indexing, and optional admin APIs.
- Keep the black-and-white newspaper style without sacrificing usability.
- Support pseudo-static URLs for posts, pages, tags, archives, and feeds.
- Support image upload, management, and paste-to-upload insertion while editing.

## Recommended Stack

- Backend: Go 1.24+.
- HTTP router: Go standard `net/http` with `ServeMux`, or `chi` if route groups and middleware composition become useful.
- Templates: Go `html/template`.
- Markdown: `github.com/yuin/goldmark`.
- Markdown extensions: GFM tables, footnotes, typographer, heading anchors.
- Math: render TeX blocks and inline TeX with KaTeX. Prefer pre-rendering at build/startup when possible; otherwise use the KaTeX auto-render extension only on article pages.
- Syntax highlighting: server-side Chroma for fastest first paint and no client JavaScript cost.
- Frontend build: no SPA framework for public pages. Use plain CSS, small progressive JavaScript, and optional `htmx` only for admin or search interactions.
- CSS: plain CSS with modern features such as cascade layers, container queries, logical properties, and `:has()` where useful.
- Storage:
  - Phase 1: file-based Markdown in `content/posts`.
  - Phase 2: SQLite for drafts, search index metadata, and admin features.
- Media storage:
  - Local filesystem in `media/` for the first version.
  - SQLite metadata for filename, hash, dimensions, alt text, caption, usage, and upload time.
  - Optional object storage later if deployment requires it.
- Search:
  - Phase 1: generated JSON index with client-side search.
  - Phase 2: SQLite FTS5.

## Speed-First Architecture

Response speed has priority over framework convenience.

Rules:

- Public pages are server-rendered HTML.
- Avoid client-side hydration for normal reading pages.
- Parse Markdown, build routes, compute tags, generate TOCs, and render article HTML at startup or during a build step.
- Keep hot request paths mostly to map lookup plus template execution.
- Cache rendered post/page bodies in memory.
- Prefer local static assets over external CDNs in production.
- Keep JavaScript optional for reading. Search and math rendering may enhance the page, but the page should remain readable without them.
- Use immutable asset filenames or strong cache headers for CSS, JS, fonts, and KaTeX assets.
- Keep template partials simple and avoid database calls from templates.
- Use SQLite only where persistence is needed; do not put article rendering behind a query-heavy path.
- Process image variants during upload, not during public page requests.
- Measure with `wrk`, `hey`, or `bombardier` before adding abstractions.

Target behavior:

- Home, post, page, tag, and archive pages should respond from memory after startup.
- Cold start may spend time indexing content; individual requests should stay cheap.
- The public site should remain useful with JavaScript disabled.

## Content Model

Postizer has three first-class content concepts:

- `Post`: dated writing, shown in feeds, archives, tag pages, and search.
- `Page`: stable standalone content, such as about, projects, links, or colophon pages.
- `Tag`: topic entity generated from posts, with optional custom metadata.

Posts are Markdown files with front matter:

```yaml
---
title: "A Note on Fourier Series"
slug: "fourier-series"
date: "2026-05-04"
updated: "2026-05-04"
tags: ["math", "signal-processing"]
summary: "Compact notes on Fourier series and convergence."
draft: false
toc: true
---
```

Main fields:

- `title`: display title.
- `slug`: canonical URL segment.
- `date`: publish date.
- `updated`: optional update date.
- `tags`: topic grouping.
- `summary`: archive/search preview.
- `draft`: excluded from public routes unless preview mode is enabled.
- `toc`: whether to show article table of contents.

Pages are Markdown files under `content/pages`:

```yaml
---
title: "About"
slug: "about"
updated: "2026-05-04"
summary: "About the author and the site."
toc: false
---
```

Tag metadata is optional. If present, store it under `content/tags`:

```yaml
---
name: "math"
title: "Mathematics"
slug: "math"
summary: "Notes involving formulas, proofs, and mathematical intuition."
---
```

If a tag metadata file does not exist, the tag should still be generated from post front matter.

## Media Model

Images are first-class managed assets, separate from posts and pages.

Recommended fields:

- `id`: stable media ID.
- `original_name`: uploaded filename.
- `stored_name`: content-hashed filename used on disk.
- `path`: public URL path, such as `/media/2026/05/hash.webp`.
- `mime_type`: detected MIME type.
- `size_bytes`: original file size.
- `width` and `height`: decoded image dimensions.
- `hash`: content hash for deduplication.
- `alt`: editable alt text.
- `caption`: optional caption.
- `uploaded_at`: upload time.
- `used_by`: post/page references, computed during indexing or stored in SQLite.

Storage layout:

```text
media/
  originals/
    2026/05/{hash}.{ext}
  public/
    2026/05/{hash}.webp
    2026/05/{hash}-small.webp
```

Default behavior:

- Preserve the original file if storage budget allows.
- Generate optimized public variants.
- Prefer WebP for public images, with the option to keep PNG/JPEG where fidelity requires it.
- Keep GIF unchanged unless animation optimization is implemented.
- Deduplicate uploads by content hash.

## Markdown And Math Rendering

Server-side Markdown rendering should:

- Parse Markdown with Goldmark.
- Enable GFM tables, strikethrough, task lists, and footnotes.
- Generate stable heading IDs for anchors.
- Sanitize rendered HTML with a strict allowlist if raw HTML is enabled.
- Preserve math delimiters for KaTeX:
  - Inline math: `$...$` or `\\(...\\)`.
  - Block math: `$$...$$` or `\\[...\\]`.
- Support LaTeX-style display equation numbering and references:
  - Automatically number every standalone display equation written as `$$...$$` or `\\[...\\]`; inline math is never numbered.
  - Put `\label{eq:name}` inside a display equation when it needs to be referenced.
  - Use `\ref{eq:name}` for the bare number and `\eqref{eq:name}` for the parenthesized form.
  - Preserve explicit `\tag{...}` values and use them as the displayed reference value.
  - Leave unresolvable references visible as `??` so drafts fail softly.

Client-side math rendering should:

- Load KaTeX CSS and the auto-render extension.
- Render only inside article content.
- Ignore code blocks, pre blocks, and script/style elements.
- Fail gracefully by leaving source TeX visible.

## Routing

Public routes:

- `GET /` latest posts, featured notes, compact archive preview.
- `GET /posts/{slug}` article page.
- `GET /pages/{slug}` standalone page.
- `GET /tags` all tags.
- `GET /tags/{tag}` posts by tag.
- `GET /archive` chronological archive.
- `GET /search` search UI.
- `GET /feed.xml` RSS feed.
- `GET /sitemap.xml` sitemap.
- `GET /media/{year}/{month}/{file}` public optimized media file.

Optional admin routes:

- `GET /admin/login` login page.
- `POST /admin/login` create signed admin session.
- `GET /admin/logout` clear admin session.
- `GET /admin` dashboard.
- `GET /admin/posts` post management page.
- `GET /admin/editor` post editor.
- `GET /admin/media` media library.
- `GET /admin/settings` site settings.
- `POST /admin/reindex` rebuild content index.

Editor routes and APIs:

- `POST /admin/api/media` upload one or more files.
- `POST /admin/api/media/paste` upload an image from clipboard paste.
- `GET /admin/api/media` list, filter, and search media.
- `GET /admin/api/media/{id}` inspect one media item.
- `PATCH /admin/api/media/{id}` rename media and update alt/caption metadata without changing the public URL.
- `DELETE /admin/api/media/{id}` remove a media item and its stored file.
- `GET /admin/api/posts` list posts.
- `GET /admin/api/posts/{slug}` get one post draft.
- `POST /admin/api/posts` create/update post.
- `DELETE /admin/api/posts/{slug}` delete one post draft or published post.
- `GET /admin/api/pages` list pages.
- `GET /admin/api/pages/{slug}` get one page draft.
- `POST /admin/api/pages` create/update page.
- `DELETE /admin/api/pages/{slug}` delete one page.
- `POST /admin/api/preview` render Markdown preview.
- `POST /admin/api/home-image` upload and enable the home image band.
- `DELETE /admin/api/home-image` clear the home image band.

Admin authentication:

- Browser access uses a signed HTTP-only session cookie.
- The login form supports a remember-me session for 30 days; otherwise the browser session is limited by the signed payload.
- Default local credentials are `admin / postizer`.
- Without `POSTIZER_SESSION_SECRET`, a local `content/.session_secret` file is created so sessions survive server restarts.
- Production credentials should be set with `POSTIZER_ADMIN_USER`, `POSTIZER_ADMIN_PASSWORD`, and `POSTIZER_SESSION_SECRET`.
- `POSTIZER_ADMIN_TOKEN` remains available for API automation through `Authorization: Bearer ...`.
- Admin API routes return `401 Unauthorized` when no valid session or token is present.

## Pseudo-Static URLs

Pseudo-static routes should return server-rendered dynamic content while exposing static-looking URLs. This keeps links familiar for older blog conventions and can help with migration from static generators.

Recommended canonical pattern:

- Posts: `/posts/{slug}`.
- Pages: `/pages/{slug}`.
- Tags: `/tags/{slug}`.
- Archive: `/archive`.

Implementation rules:

- Serve only the clean extensionless public URLs.
- Emit extensionless URLs in templates, RSS, sitemap, search index, and admin view links.
- Do not add `.html` compatibility redirects unless a migration explicitly requires them.
- Validate slugs as URL-safe title segments made from letters, numbers, and hyphens.
- Do not map arbitrary public paths directly to the filesystem.

## Backend Structure

```text
cmd/postizer/main.go
internal/config/
internal/http/
internal/content/
internal/render/
internal/search/
internal/media/
internal/feed/
web/templates/
web/static/
content/posts/
content/pages/
media/
```

Responsibilities:

- `config`: load environment and site config.
- `http`: routes, middleware, handlers.
- `content`: read Markdown files, parse front matter, build in-memory index.
- `content`: manage posts, pages, and generated or explicit tags.
- `render`: Markdown, syntax highlighting, TOC extraction.
- `search`: build search documents and expose search data.
- `media`: upload validation, image decoding, hashing, resizing, format conversion, metadata, and usage tracking.
- `feed`: RSS and sitemap generation.

## Image Upload And Management

Admin media management should support:

- Drag-and-drop image upload.
- File picker upload.
- Clipboard paste upload while editing a post or page.
- Media library grid/list with filename, dimensions, size, type, upload date, and usage status.
- Upload images directly from the media library page.
- Search by filename, alt text, caption, MIME type, and usage.
- Rename media display names and edit alt text and captions without changing public URLs.
- Delete media items from the library and filesystem.
- Copy Markdown snippet.
- Insert selected image at the current editor cursor position.
- Detect unused media and show where each asset is referenced.

Paste-to-upload workflow:

1. User copies an image from screenshot tools, browser, filesystem, or design software.
2. In the Markdown editor, user presses `Ctrl+V` or `Cmd+V`.
3. The editor reads image data from `clipboardData.items`.
4. The admin frontend sends it to `POST /admin/api/media/paste` as `multipart/form-data`.
5. The server validates, hashes, stores, optimizes, and records metadata.
6. The API returns a figure snippet.
7. The editor inserts the snippet at the current cursor position.

Returned snippet:

```markdown
\begin{figure}
![Alt text](/media/2026/05/hash.webp)
\caption{Caption text}
\label{fig:example}
\end{figure}
```

The renderer should convert figure blocks to semantic HTML with `figure`, `img`, and `figcaption`. All standalone Markdown images are also rendered as numbered figures, so public article content has one image model.

Figure references:

- Use `\figref{fig:example}` for `Figure 1`.
- Use `\ref{fig:example}` for the bare number.
- Figure labels use the `fig:` namespace; equation labels use `eq:`.
- Render article figures at 60% width on desktop, full width on narrow screens, and open a large image viewer on click.

Upload validation:

- Accept `image/jpeg`, `image/png`, `image/gif`, and `image/webp`.
- Reject SVG by default until sanitization exists.
- Enforce file size limits.
- Decode images server-side to verify actual type and dimensions.
- Strip unsafe metadata where possible.
- Generate dimensions at upload time to avoid layout shift.
- Use content hashes to prevent duplicate storage.

Recommended first implementation:

- JPEG, PNG, GIF, and WebP uploads.
- WebP optimized output for JPEG/PNG/WebP.
- Keep GIF as original unless animation handling is implemented.
- Store metadata in SQLite.
- Insert figure blocks from upload, paste upload, and media library actions.

## Editor Experience

The admin editor should feel like a complete publishing desk, not a plain Markdown textarea.

Admin shell:

- Use a dedicated backend layout separate from the public newspaper shell.
- Sidebar navigation links to Dashboard, Posts, Editor, Media, and Settings.
- Each backend section is a separate page so future management features can grow without crowding the editor.

Core areas:

- Top action bar: current save state, new post, delete, save draft, publish, and view published URL.
- Post library: compact list of existing posts with date and draft/published state.
- Writing surface: large title input, automatic pseudo-static permalink display, Markdown toolbar, source editor, and live preview.
- Document settings: date, updated date, tags, summary, draft state, and TOC toggle.
- Media panel: recent media thumbnails, direct upload, click-to-insert, and paste-to-upload from the editor.

Editor behavior:

- Generate the slug from the title on every save, replacing separators with hyphens and adding a numeric suffix when needed.
- Save posts back to `content/posts/{slug}.md`.
- Delete posts and pages from their content folders, then reload the in-memory index.
- Reload the in-memory content index after saving.
- Render preview through the same server-side Markdown pipeline as public pages.
- Support edit-only, split, and preview modes.
- Keep the UI black-and-white and dense, but make controls comparable to modern blog editors.

## Frontend Methodology

Frontend implementation should start from a basic component library, then compose pages from those primitives.

Sequence:

- Define design tokens first: colors, fonts, borders, spacing, shell width, and states.
- Build reusable primitives with a `ui-*` prefix.
- Build domain composites such as `editor-*` from those primitives.
- Keep page-specific CSS focused on layout and behavior, not reinventing controls.
- Prefer server-rendered HTML and progressive JavaScript.
- Verify responsive behavior and text fitting before considering the UI complete.

Base primitives:

- `ui-button`, `ui-button--primary`, `ui-button--ghost`.
- `ui-status`.
- `ui-panel`, `ui-panel__head`.
- `ui-field`, `ui-check`, `ui-input`, `ui-textarea`.
- `ui-toolbar`, `ui-separator`.
- `ui-list-button`.
- `ui-media-button`.

The detailed frontend methodology lives in `docs/FRONTEND.md`.

## Page Layout

Global layout:

- Overall page shell is centered and width-constrained instead of full-bleed.
- Desktop keeps side breathing room only; the shell touches the top and bottom viewport edges without a bottom border.
- Top masthead: edition line plus large `Postizer` title.
- Section navigation: horizontal newspaper-style section bar below the masthead.
- Main column: article, front page, or listing content.
- Right column: topic index, pages, feeds, article metadata, or related links.
- Mobile: masthead and section navigation remain first; content and side column stack vertically.

Front page:

- Optional home image band appears above the lead story when configured.
- Home image uses an uploaded media asset and is visually cropped with fixed responsive height.
- Lead story occupies the primary left column.
- Latest reports appear as a compact right-side news index.
- Tags, pages, and feeds remain in the global right column.
- Avoid app-like left navigation rails on public pages.

Article page:

- Dense header with title, date, tags, reading time, update date.
- Main article body with strong typographic hierarchy.
- Article Markdown headings follow a fixed editorial hierarchy:
  - Level 1: 14pt bold, all caps.
  - Level 2: 12pt bold, source-authored title case.
  - Level 3: 12pt italic, source-authored title case.
  - Level 4: 12pt normal, source-authored sentence case.
  - Level 5: 12pt normal until a stricter rule is defined.
- Article body text uses 12pt with compact `line-height: 1.4`.
- Article body should use the full available main column width in a single column.
- Right column can hold table of contents, article metadata, backlinks, or related posts.
- Footnotes, references, and previous/next links.

Listing pages:

- Compact rows instead of large cards.
- Date, title, tags, and one-line summary.
- Keyboard-friendly search/filter input.

## Newspaper Visual Direction

Use a restrained black-and-white newspaper/editorial language inspired by traditional broadsheet layouts:

- Background: white paper.
- Text: black ink.
- Accent colors: none for the public theme; use black, white, and grayscale only.
- Typeface:
  - English body: `Times New Roman`, 12pt in article content.
  - Chinese body: Songti/宋体, sized according to Chinese core-journal style, with article body at 12pt.
  - Chinese headings: Heiti/黑体.
  - UI/meta: monospace.
  - Code/math: monospace and KaTeX defaults.
- Borders: 1px solid, square or 4px radius at most.
- Texture: avoid colored grain; use clean whitespace, rules, columns, and typographic contrast instead.
- Layout density: compact rows, small metadata, visible dividers, minimal whitespace.
- Interaction: modern responsiveness and accessibility while preserving the newspaper-like editorial surface.
- Article heading hierarchy follows a numbered journal/LaTeX-like scale:
  - Use automatic hierarchical numbering such as `1`, `1.1`, and `1.1.1`.
  - Keep a fixed, narrow gap after the heading number; title text follows the natural width of numbers like LaTeX section headings.
  - Headings must not be smaller than the 12pt article body.
  - H1/H2/H3 rely on descending size and bold weight for hierarchy.
  - H4/H5/H6 remain at least 12pt and rely on deeper numbering and compact spacing.

Avoid making the interface look like a landing page. The first screen should immediately show real posts, tags, archive links, and search.

## CSS Architecture

- Use plain modern CSS. Avoid heavy UI frameworks for the public theme.
- Define design tokens in `:root`.
- Put base, layout, components, and utilities in cascade layers.
- Prefer semantic classes: `.site-shell`, `.post-list`, `.post-meta`, `.article-body`.
- Keep content typography isolated under `.article-body`.
- Use responsive grid:

```css
.site-shell {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 15rem;
  grid-template-areas:
    "header header"
    "nav nav"
    "main side";
  width: min(100%, 96rem);
  margin-inline: auto;
  border: 1px solid var(--rule);
}
```

At smaller widths, remove the outer border, stack masthead, section navigation, content, and side column.

Base typography should explicitly separate Latin, Chinese body, and Chinese heading fallbacks:

```css
:root {
  --font-latin: "Times New Roman", Times, serif;
  --font-cjk-body: SimSun, "宋体", STSong, "Songti SC", serif;
  --font-cjk-heading: SimHei, "黑体", "Microsoft YaHei", "Heiti SC", sans-serif;
  --font-body: var(--font-latin), var(--font-cjk-body);
  --font-heading: var(--font-latin), var(--font-cjk-heading);
  --font-ui: "Courier New", Courier, monospace;
}

body {
  font-family: var(--font-body);
}

.site-nav,
.post-meta,
.tag-list,
.toc,
code,
pre {
  font-family: var(--font-ui);
}
```

## Security

- Treat Markdown as untrusted unless only trusted authors can publish.
- Sanitize HTML or disable raw HTML in Markdown.
- Use strict Content Security Policy.
- Validate slugs and file paths to prevent path traversal.
- Validate upload paths and never trust client-provided filenames.
- Keep admin routes behind authentication.
- Use CSRF protection for admin POST requests.
- Limit upload size and decode images before exposing them publicly.
- Sanitize or reject SVG uploads.

## Performance

- Build the content index at startup or during a build command.
- Cache rendered Markdown, TOCs, tag indexes, archive pages, and search metadata in memory.
- Use `ETag` or `Last-Modified` headers for static and article pages.
- Keep CSS small and avoid large client frameworks for the reader UI.
- Use KaTeX only where article content exists, and prefer pre-rendered math for frequently visited posts.
- Compress responses with gzip or brotli when served behind a proxy that supports it.
- Serve static assets with long cache lifetimes and content-hashed filenames.
- Serve optimized media directly from disk or through a reverse proxy static file path.
- Add lightweight request timing logs so slow routes are visible early.

## Implementation Phases

1. Static blog core:
   - File-based Markdown posts.
   - Public routes.
   - Templates and black-and-white dense CSS.
   - RSS and sitemap.

2. Writing quality:
   - Tags, archive, search index.
   - TOC extraction.
   - Syntax highlighting.
   - KaTeX auto-render.

3. Admin and persistence:
   - SQLite.
   - Draft preview.
   - Authenticated editor.
   - Media uploads.
   - Paste-to-upload image insertion.
   - Media library and asset usage tracking.
   - Modern editor workspace with post library, document settings, live preview, and publishing actions.

4. Advanced knowledge features:
   - Backlinks.
   - Related posts.
   - Full-text search.
   - Series support.

## Suggested First Build

Start with a minimal server-rendered Go application:

- Go 1.24+ `net/http` plus templates.
- Goldmark for Markdown.
- Front matter parser.
- File-based posts under `content/posts`.
- KaTeX loaded from local static assets, with a path toward startup pre-rendering.
- One modern plain CSS file implementing the black-and-white dense newspaper layout.

This gives a usable blog quickly while keeping the path open for admin features and SQLite later.
