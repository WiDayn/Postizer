const root = document.querySelector(".editor-shell");
const editor = document.querySelector("#markdownEditor");
const previewPane = document.querySelector("#previewPane");
const uploadForm = document.querySelector("#mediaUpload");
const fileInput = document.querySelector("#fileInput");
const homeImageUpload = document.querySelector("#homeImageUpload");
const homeImageInput = document.querySelector("#homeImageInput");
const clearHomeImageButton = document.querySelector("#clearHomeImageButton");
const statusLine = document.querySelector("#editorStatus");
const saveState = document.querySelector("#saveState");
const wordCount = document.querySelector("#wordCount");
const postLibrary = document.querySelector("#postLibrary");
const postLibraryToggle = document.querySelector("#togglePostLibrary");
const deletePostButton = document.querySelector("#deletePostButton");
const mediaStrip = document.querySelector("#mediaStrip");
const mediaPager = document.querySelector("#mediaPager");
const mediaPrev = document.querySelector("#mediaPrev");
const mediaNext = document.querySelector("#mediaNext");
const mediaPageInfo = document.querySelector("#mediaPageInfo");
const contentKind = root && root.dataset.contentKind === "page" ? "page" : "post";
const isPageEditor = contentKind === "page";
const contentPlural = isPageEditor ? "pages" : "posts";
const apiBase = `/admin/api/${contentPlural}`;
const adminBase = `/admin/${contentPlural}`;
const permalinkPattern = String((root && root.dataset.permalinkPattern) || (isPageEditor ? "/pages/%pagename%" : "/posts/%postname%"));
const initialSlug = (root && root.dataset.initialSlug) || "";
const fields = {
  title: document.querySelector("#postTitle"),
  slug: document.querySelector("#postSlug"),
  date: document.querySelector("#postDate"),
  updated: document.querySelector("#postUpdated"),
  tags: document.querySelector("#postTags"),
  summary: document.querySelector("#postSummary"),
  draft: document.querySelector("#postDraft"),
  toc: document.querySelector("#postTOC"),
  view: document.querySelector("#viewPostLink")
};

let currentSlug = "";
let updatedTouched = false;
let previewTimer = 0;
let dirty = false;
const postsCollapsedKey = `postizer.editor.${contentPlural}Collapsed`;
const postLibraryBatchSize = Math.max(10, Number(postLibrary && postLibrary.dataset.batchSize) || 40);
const mediaPageSize = Math.max(1, Number(mediaStrip && mediaStrip.dataset.pageSize) || 12);
let postLibraryItems = [];
let postLibraryRendered = 0;
let postLibraryActiveSlug = "";
let mediaItems = [];
let mediaPage = 1;

function tr(key, fallback) {
  if (window.postizerMessage) return window.postizerMessage(key, fallback);
  return fallback;
}

function formatMessage(key, fallback, replacements = {}) {
  if (window.postizerFormatMessage) return window.postizerFormatMessage(key, fallback, replacements);
  let text = tr(key, fallback);
  Object.entries(replacements).forEach(([name, value]) => {
    text = text.replaceAll(`{${name}}`, String(value));
  });
  return text;
}

function apiURL(path) {
  const token = new URLSearchParams(window.location.search).get("token");
  if (!token) return path;
  const url = new URL(path, window.location.origin);
  url.searchParams.set("token", token);
  return url.pathname + url.search;
}

function setStatus(message) {
  if (statusLine) statusLine.textContent = message;
}

function setDirty(value) {
  dirty = value;
  if (saveState) saveState.textContent = dirty ? tr("editor.unsaved", "Unsaved") : tr("editor.saved", "Saved");
}

function setPostLibraryCollapsed(collapsed) {
  if (!root || !postLibraryToggle) return;
  root.dataset.library = collapsed ? "collapsed" : "expanded";
  postLibraryToggle.setAttribute("aria-expanded", String(!collapsed));
  const expandFallback = isPageEditor ? "Expand pages" : "Expand posts";
  const collapseFallback = isPageEditor ? "Collapse pages" : "Collapse posts";
  postLibraryToggle.title = collapsed ? tr("editor.library.expand", expandFallback) : tr("editor.library.collapse", collapseFallback);
  postLibraryToggle.textContent = collapsed ? ">" : "<";
  try {
    localStorage.setItem(postsCollapsedKey, collapsed ? "1" : "0");
  } catch (_) {
    // Browsers can disable storage; the toggle still works for the current page.
  }
}

function restorePostLibraryState() {
  try {
    setPostLibraryCollapsed(localStorage.getItem(postsCollapsedKey) === "1");
  } catch (_) {
    setPostLibraryCollapsed(false);
  }
}

function siteTimeZone() {
  return (root && root.dataset.timeZone) || "UTC";
}

function nowMinute() {
  const date = new Date();
  try {
    const parts = new Intl.DateTimeFormat("en-CA", {
      timeZone: siteTimeZone(),
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23"
    }).formatToParts(date).reduce((acc, part) => {
      acc[part.type] = part.value;
      return acc;
    }, {});
    return `${parts.year}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}`;
  } catch (_) {
    const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
    return local.toISOString().slice(0, 16);
  }
}

function displayDateTime(value) {
  return String(value || "").replace("T", " ");
}

function unicodeRegExp(source, flags, fallback) {
  try {
    return new RegExp(source, flags);
  } catch (_) {
    return fallback;
  }
}

const slugSeparatorPattern = unicodeRegExp(
  "[^\\p{L}\\p{N}]+",
  "gu",
  /[^0-9a-z\u3007\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\u3040-\u30ff\uac00-\ud7af]+/gi
);
const wordPattern = unicodeRegExp(
  "[\\p{L}\\p{N}_]+",
  "gu",
  /[0-9a-z_\u3007\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\u3040-\u30ff\uac00-\ud7af]+/gi
);

function slugify(value) {
  return String(value || "")
    .toLowerCase()
    .trim()
    .replace(slugSeparatorPattern, "-")
    .replace(/^-+|-+$/g, "")
    .replace(/-+/g, "-");
}

function titleSlug() {
  return slugify(fields.title.value);
}

function visibleSlug() {
  return titleSlug() || currentSlug;
}

function setSlugDisplay(slug) {
  if (fields.slug) fields.slug.textContent = publicURLForSlug(slug || "").replace(/^\//, "");
}

function datePartsForPermalink() {
  const date = String((fields.date && fields.date.value) || "");
  const match = date.match(/^(\d{4})-(\d{2})-(\d{2})/);
  return {
    year: match ? match[1] : "0000",
    monthnum: match ? match[2] : "00",
    day: match ? match[3] : "00"
  };
}

function publicURLForSlug(slug) {
  const parts = datePartsForPermalink();
  const replacements = {
    "%postname%": slug,
    "%pagename%": slug,
    "%slug%": slug,
    "%year%": parts.year,
    "%monthnum%": parts.monthnum,
    "%day%": parts.day
  };
  let path = permalinkPattern.replace(/%([A-Za-z0-9_]+)%/g, (token, name) => {
    const key = `%${String(name || "").toLowerCase()}%`;
    return Object.prototype.hasOwnProperty.call(replacements, key) ? replacements[key] : token;
  });
  if (!path.startsWith("/")) path = `/${path}`;
  return path;
}

function updateDeleteButton() {
  if (deletePostButton) deletePostButton.hidden = !currentSlug;
}

function insertAtCursor(textarea, text) {
  const start = textarea.selectionStart || 0;
  const end = textarea.selectionEnd || 0;
  textarea.value = textarea.value.slice(0, start) + text + textarea.value.slice(end);
  textarea.selectionStart = textarea.selectionEnd = start + text.length;
  textarea.focus();
  afterEdit();
}

function insertDisplayEquation() {
  const start = editor.selectionStart || 0;
  const end = editor.selectionEnd || 0;
  const selected = editor.value.slice(start, end).trim() || "E = mc^2";
  const label = `eq:${slugify(fields.title.value || "equation") || "equation"}`;
  const before = editor.value.slice(0, start);
  const after = editor.value.slice(end);
  const prefix = before && !before.endsWith("\n\n") ? (before.endsWith("\n") ? "\n" : "\n\n") : "";
  const suffix = after && !after.startsWith("\n\n") ? (after.startsWith("\n") ? "\n" : "\n\n") : "";
  const snippet = `${prefix}$$\n${selected}\n\\label{${label}}\n$$${suffix}`;
  editor.value = before + snippet + after;
  const labelStart = before.length + snippet.indexOf(label);
  editor.selectionStart = labelStart;
  editor.selectionEnd = labelStart + label.length;
  editor.focus();
  afterEdit();
}

function markdownImageAlt(value) {
  return String(value || tr("editor.image.default_alt", "Image")).replace(/\\/g, "\\\\").replace(/\]/g, "\\]");
}

function latexBraceText(value) {
  return String(value || tr("editor.image.default_alt", "Image")).replace(/\\/g, "\\\\").replace(/[{}]/g, "");
}

function nextFigureLabel(path = "") {
  const base = titleSlug() || currentSlug || "figure";
  const count = (editor.value.match(/\\label\{fig:/g) || []).length + 1;
  const file = slugify((path.split("/").pop() || "").replace(/\.[^.]+$/, ""));
  return `fig:${base}-${file || count}`;
}

function figureMarkdown(item = {}) {
  const path = item.path || item.Path || "";
  const alt = item.alt || item.Alt || tr("editor.image.default_alt", "Image");
  const caption = item.caption || item.Caption || alt;
  const label = nextFigureLabel(path);
  return `\n\n\\begin{figure}\n![${markdownImageAlt(alt)}](${path})\n\\caption{${latexBraceText(caption)}}\n\\label{${label}}\n\\end{figure}\n\n`;
}

function markdownBlock(markdown = "") {
  const trimmed = String(markdown || "").trim();
  return trimmed ? `\n\n${trimmed}\n\n` : "";
}

function markdownLinkText(value) {
  return String(value || tr("media.file.default_label", "File")).replace(/\\/g, "\\\\").replace(/\]/g, "\\]");
}

function markdownLinkDestination(value) {
  return String(value || "").replace(/\\/g, "%5C").replace(/\(/g, "%28").replace(/\)/g, "%29");
}

function isImageItem(item = {}) {
  return String(item.mime_type || item.MIMEType || item.mimeType || "").toLowerCase().startsWith("image/");
}

function mediaFileLabel(item = {}) {
  const path = item.path || item.Path || "";
  return item.caption || item.Caption || item.alt || item.Alt || item.original_name || item.OriginalName || path.split("/").pop() || tr("media.file.default_label", "File");
}

function mediaFileType(item = {}) {
  const value = String(item.original_name || item.OriginalName || item.path || item.Path || "");
  const match = value.match(/\.([^./\\]+)$/);
  return match ? match[1].toUpperCase() : "FILE";
}

function mediaMarkdown(item = {}) {
  const path = item.path || item.Path || "";
  if (!path) return "";
  if (isImageItem(item)) return figureMarkdown(item);
  return `\n\n[${markdownLinkText(mediaFileLabel(item))}](${markdownLinkDestination(path)})\n\n`;
}

function postLibraryItemFromButton(button) {
  return {
    slug: button.dataset.slug || "",
    title: (button.querySelector("strong") && button.querySelector("strong").textContent) || "",
    meta: (button.querySelector("span") && button.querySelector("span").textContent) || ""
  };
}

function renderPostLibraryItem(item = {}) {
  const li = document.createElement("li");
  const button = document.createElement("button");
  button.type = "button";
  button.className = "ui-list-button";
  button.dataset.slug = item.slug || "";
  button.classList.toggle("is-active", item.slug === postLibraryActiveSlug);
  button.innerHTML = "<strong></strong><span></span>";
  button.querySelector("strong").textContent = item.title || "";
  button.querySelector("span").textContent = item.meta || "";
  li.appendChild(button);
  return li;
}

function renderPostLibraryEmpty() {
  const li = document.createElement("li");
  li.className = "empty";
  li.textContent = postLibrary.dataset.emptyText || tr("editor.library.empty", "No posts");
  postLibrary.appendChild(li);
}

function appendPostLibraryItems(count = postLibraryBatchSize) {
  if (!postLibrary) return;
  if (!postLibraryItems.length) {
    postLibrary.innerHTML = "";
    renderPostLibraryEmpty();
    return;
  }
  const start = postLibraryRendered;
  const end = Math.min(postLibraryItems.length, start + count);
  if (start >= end) return;
  const fragment = document.createDocumentFragment();
  postLibraryItems.slice(start, end).forEach((item) => {
    fragment.appendChild(renderPostLibraryItem(item));
  });
  postLibrary.appendChild(fragment);
  postLibraryRendered = end;
}

function updatePostLibraryActive(slug = postLibraryActiveSlug) {
  postLibraryActiveSlug = slug || "";
  if (!postLibrary) return;
  postLibrary.querySelectorAll("button[data-slug]").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.slug === postLibraryActiveSlug);
  });
}

function renderPostLibrary(activeSlug = postLibraryActiveSlug) {
  if (!postLibrary) return;
  postLibraryActiveSlug = activeSlug || "";
  postLibrary.innerHTML = "";
  postLibraryRendered = 0;
  appendPostLibraryItems(postLibraryBatchSize);
  updatePostLibraryActive(postLibraryActiveSlug);
}

function captureInitialPostLibrary() {
  if (!postLibrary) return;
  postLibraryItems = Array.from(postLibrary.querySelectorAll("button[data-slug]")).map(postLibraryItemFromButton);
  renderPostLibrary(initialSlug);
}

function maybeLoadMorePostLibrary() {
  if (!postLibrary || postLibraryRendered >= postLibraryItems.length) return;
  const remaining = postLibrary.scrollHeight - postLibrary.scrollTop - postLibrary.clientHeight;
  if (remaining < 120) appendPostLibraryItems(postLibraryBatchSize);
}

function normalizeMediaItem(item = {}) {
  return {
    path: item.path || item.Path || "",
    mime_type: item.mime_type || item.MIMEType || item.mimeType || "",
    markdown: item.markdown || item.Markdown || "",
    alt: item.alt || item.Alt || "",
    caption: item.caption || item.Caption || "",
    original_name: item.original_name || item.OriginalName || ""
  };
}

function mediaItemFromButton(button) {
  return normalizeMediaItem({
    path: button.dataset.path,
    mime_type: button.dataset.mimeType,
    markdown: button.dataset.markdown,
    alt: button.dataset.alt,
    caption: button.dataset.caption,
    original_name: button.title
  });
}

function renderMediaButton(item = {}) {
  const normalized = normalizeMediaItem(item);
  const button = document.createElement("button");
  button.type = "button";
  button.className = "ui-media-button";
  button.dataset.path = normalized.path;
  button.dataset.mimeType = normalized.mime_type || "";
  button.dataset.markdown = isImageItem(normalized) ? "" : (normalized.markdown || mediaMarkdown(normalized));
  button.dataset.alt = normalized.alt || tr("editor.image.default_alt", "Image");
  button.dataset.caption = normalized.caption || "";
  button.title = normalized.original_name || normalized.path;
  if (isImageItem(normalized)) {
    const img = document.createElement("img");
    img.src = normalized.path;
    img.alt = normalized.alt || "";
    button.appendChild(img);
  } else {
    const fileBadge = document.createElement("span");
    fileBadge.className = "ui-media-button__file";
    fileBadge.textContent = mediaFileType(normalized);
    button.appendChild(fileBadge);
  }
  return button;
}

function renderMediaPage(page = mediaPage) {
  if (!mediaStrip) return;
  const total = mediaItems.length;
  const totalPages = Math.max(1, Math.ceil(total / mediaPageSize));
  mediaPage = Math.min(Math.max(1, page), totalPages);
  mediaStrip.innerHTML = "";
  if (!total) {
    const empty = document.createElement("p");
    empty.className = "media-strip-empty";
    empty.textContent = mediaStrip.dataset.emptyText || tr("editor.settings.no_media", "No media");
    mediaStrip.appendChild(empty);
  } else {
    const start = (mediaPage - 1) * mediaPageSize;
    mediaItems.slice(start, start + mediaPageSize).forEach((item) => {
      mediaStrip.appendChild(renderMediaButton(item));
    });
  }
  if (mediaPager) mediaPager.hidden = total <= mediaPageSize;
  if (mediaPrev) mediaPrev.disabled = mediaPage <= 1;
  if (mediaNext) mediaNext.disabled = mediaPage >= totalPages;
  if (mediaPageInfo) {
    mediaPageInfo.textContent = formatMessage("editor.media.page_info", "Page {page} / {pages}", {
      page: mediaPage,
      pages: totalPages
    });
  }
}

function captureInitialMedia() {
  if (!mediaStrip) return;
  mediaItems = Array.from(mediaStrip.querySelectorAll("button[data-path]")).map(mediaItemFromButton);
  renderMediaPage(1);
}

function selectionWrap(before, after = before, fallback = "") {
  const start = editor.selectionStart || 0;
  const end = editor.selectionEnd || 0;
  const selected = editor.value.slice(start, end) || fallback;
  editor.value = editor.value.slice(0, start) + before + selected + after + editor.value.slice(end);
  editor.selectionStart = start + before.length;
  editor.selectionEnd = start + before.length + selected.length;
  editor.focus();
  afterEdit();
}

function prefixSelection(prefix) {
  const start = editor.selectionStart || 0;
  const end = editor.selectionEnd || 0;
  const selected = editor.value.slice(start, end) || tr("editor.selection.default_text", "Text");
  const next = selected.split("\n").map((line) => `${prefix}${line}`).join("\n");
  editor.value = editor.value.slice(0, start) + next + editor.value.slice(end);
  editor.selectionStart = start;
  editor.selectionEnd = start + next.length;
  editor.focus();
  afterEdit();
}

function applyCommand(command) {
  const commands = {
    bold: () => selectionWrap("**", "**", "bold text"),
    italic: () => selectionWrap("_", "_", "italic text"),
    heading: () => prefixSelection("## "),
    quote: () => prefixSelection("> "),
    code: () => selectionWrap("`", "`", "code"),
    link: () => selectionWrap("[", "](https://example.com)", "link"),
    math: () => selectionWrap("\\(", "\\)", "x^2"),
    equation: () => insertDisplayEquation(),
    eqref: () => selectionWrap("\\eqref{", "}", "eq:name"),
    figref: () => selectionWrap("\\figref{", "}", "fig:name"),
    image: () => fileInput && fileInput.click()
  };
  if (commands[command]) commands[command]();
}

function payload(draftOverride) {
  const data = {
    title: fields.title.value.trim(),
    slug: titleSlug(),
    original_slug: currentSlug,
    date: fields.date.value || nowMinute(),
    updated: fields.updated.value,
    updated_manual: updatedTouched,
    summary: fields.summary.value.trim(),
    toc: fields.toc.checked,
    draft: typeof draftOverride === "boolean" ? draftOverride : fields.draft.checked,
    body: editor.value
  };
  if (!isPageEditor) {
    data.tags = fields.tags.value.split(",").map((tag) => tag.trim()).filter(Boolean);
  }
  return data;
}

function fillForm(post) {
  currentSlug = post.slug || "";
  updatedTouched = false;
  fields.title.value = post.title || "";
  fields.date.value = post.date || nowMinute();
  fields.updated.value = post.updated || "";
  fields.tags.value = isPageEditor ? "" : (post.tags || []).join(", ");
  fields.summary.value = post.summary || "";
  fields.draft.checked = Boolean(post.draft);
  fields.toc.checked = post.toc !== false;
  editor.value = post.body || "";
  updateDeleteButton();
  updateViewLink();
  updateCounts();
  schedulePreview(true);
  setDirty(false);
}

async function loadPost(slug) {
  setStatus(tr("editor.status.loading", "Loading"));
  const response = await fetch(apiURL(`${apiBase}/${slug}`));
  if (!response.ok) throw new Error(await response.text());
  fillForm(await response.json());
  updatePostLibraryActive(slug);
  if (slug) history.replaceState(null, "", `${adminBase}/${slug}/edit`);
  setStatus(tr("editor.status.ready", "Ready"));
}

function newPost() {
  fillForm({
    title: "",
    slug: "",
    date: nowMinute(),
    updated: "",
    tags: [],
    summary: "",
    draft: true,
    toc: true,
    body: "## Notes\n\n"
  });
  currentSlug = "";
  updatePostLibraryActive("");
  history.replaceState(null, "", `${adminBase}/new`);
  fields.title.focus();
  setStatus(tr("editor.status.new", "New"));
  setDirty(true);
}

async function savePost(draft) {
  const data = payload(draft);
  if (!data.title) {
    fields.title.focus();
    setStatus(tr("editor.status.title_required", "Title required"));
    return;
  }
  setStatus(draft ? tr("editor.status.saving_draft", "Saving draft") : tr("editor.status.publishing", "Publishing"));
  const response = await fetch(apiURL(apiBase), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data)
  });
  if (!response.ok) {
    setStatus((await response.text()).trim());
    return;
  }
  const result = await response.json();
  currentSlug = result.slug;
  setSlugDisplay(result.slug);
  updateDeleteButton();
  fields.draft.checked = Boolean(result.draft);
  if (result.date) fields.date.value = result.date;
  if (result.updated) fields.updated.value = result.updated;
  updatedTouched = false;
  updateViewLink();
  history.replaceState(null, "", `${adminBase}/${result.slug}/edit`);
  await refreshPostLibrary(result.slug);
  setDirty(false);
  setStatus(result.draft ? tr("editor.status.draft_saved", "Draft saved") : tr("editor.status.published", "Published"));
}

async function deleteCurrentPost() {
  if (!currentSlug) return;
  const confirmKey = isPageEditor ? "pages.confirm.delete" : "posts.confirm.delete";
  const confirmFallback = isPageEditor ? "Delete this page?" : "Delete this post?";
  if (!window.confirm(tr(confirmKey, confirmFallback))) return;
  setStatus(isPageEditor ? tr("editor.status.deleting_page", "Deleting page") : tr("editor.status.deleting_post", "Deleting post"));
  const response = await fetch(apiURL(`${apiBase}/${encodeURIComponent(currentSlug)}`), { method: "DELETE" });
  if (!response.ok) {
    setStatus((await response.text()).trim());
    return;
  }
  setDirty(false);
  window.location.href = adminBase;
}

async function refreshPostLibrary(activeSlug) {
  const response = await fetch(apiURL(apiBase));
  if (!response.ok) return;
  const posts = await response.json();
  postLibraryItems = Array.isArray(posts) ? posts.map((post) => ({
    slug: post.slug || "",
    title: post.title || "",
    meta: `${displayDateTime(post.date)} ${post.draft ? tr("common.draft", "Draft") : tr("common.published", "Published")}`
  })) : [];
  renderPostLibrary(activeSlug);
}

async function updatePreview() {
  const response = await fetch(apiURL("/admin/api/preview"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ markdown: editor.value })
  });
  if (!response.ok) return;
  const result = await response.json();
  previewPane.innerHTML = result.html || "";
  if (window.postizerEnhanceArticle) {
    window.postizerEnhanceArticle(previewPane);
  } else if (window.renderMathInElement) {
    renderMathInElement(previewPane, {
      delimiters: [
        { left: "$$", right: "$$", display: true },
        { left: "\\[", right: "\\]", display: true },
        { left: "\\(", right: "\\)", display: false },
        { left: "$", right: "$", display: false }
      ],
      ignoredTags: ["script", "noscript", "style", "textarea", "pre", "code"],
      throwOnError: false
    });
  }
}

function schedulePreview(now = false) {
  clearTimeout(previewTimer);
  previewTimer = setTimeout(updatePreview, now ? 0 : 250);
}

function updateCounts() {
  const matches = editor.value.match(wordPattern);
  if (wordCount) wordCount.textContent = formatMessage("editor.word_count", "{count} words", { count: matches ? matches.length : 0 });
}

function updateViewLink() {
  const slug = visibleSlug();
  setSlugDisplay(slug);
  fields.view.href = slug ? publicURLForSlug(slug) : "/";
}

function afterEdit() {
  updateCounts();
  schedulePreview();
  setDirty(true);
}

async function uploadFile(file, endpoint = "/admin/api/media") {
  const data = new FormData();
  data.append("file", file, file.name || "pasted-image.png");
  setStatus(tr("editor.status.uploading", "Uploading"));
  const response = await fetch(apiURL(endpoint), { method: "POST", body: data });
  if (!response.ok) throw new Error(await response.text());
  const result = await response.json();
  const item = result.item || {};
  setStatus(tr("editor.status.uploaded", "Uploaded"));
  await refreshMedia();
  return !isImageItem(item) && result.markdown ? markdownBlock(result.markdown) : mediaMarkdown(item);
}

async function refreshMedia() {
  const response = await fetch(apiURL("/admin/api/media"));
  if (!response.ok) return;
  const items = await response.json();
  mediaItems = Array.isArray(items) ? items.map(normalizeMediaItem) : [];
  renderMediaPage(1);
}

document.querySelectorAll("[data-command]").forEach((button) => {
  button.addEventListener("click", () => applyCommand(button.dataset.command));
});

document.querySelectorAll("[data-mode-button]").forEach((button) => {
  button.addEventListener("click", () => {
    root.dataset.mode = button.dataset.modeButton;
    document.querySelectorAll("[data-mode-button]").forEach((item) => {
      item.classList.toggle("is-active", item === button);
    });
    if (button.dataset.modeButton !== "edit") schedulePreview(true);
  });
});

if (postLibraryToggle) {
  restorePostLibraryState();
  postLibraryToggle.addEventListener("click", () => {
    setPostLibraryCollapsed(root.dataset.library !== "collapsed");
  });
}

captureInitialPostLibrary();

if (postLibrary) {
  postLibrary.addEventListener("scroll", maybeLoadMorePostLibrary);
}

captureInitialMedia();

if (mediaPrev) {
  mediaPrev.addEventListener("click", () => renderMediaPage(mediaPage - 1));
}

if (mediaNext) {
  mediaNext.addEventListener("click", () => renderMediaPage(mediaPage + 1));
}

postLibrary.addEventListener("click", (event) => {
  const button = event.target.closest("button[data-slug]");
  if (!button) return;
  loadPost(button.dataset.slug).catch((error) => setStatus(error.message));
});

mediaStrip.addEventListener("click", (event) => {
  const button = event.target.closest("button[data-path]");
  if (button) {
    if (String(button.dataset.mimeType || "").toLowerCase().startsWith("image/")) {
      insertAtCursor(editor, figureMarkdown({
        path: button.dataset.path,
        alt: button.dataset.alt,
        caption: button.dataset.caption
      }));
      return;
    }
    const markdown = button.dataset.markdown || mediaMarkdown({
      path: button.dataset.path,
      mime_type: button.dataset.mimeType,
      alt: button.dataset.alt,
      caption: button.dataset.caption
    });
    insertAtCursor(editor, markdownBlock(markdown));
  }
});

fields.title.addEventListener("input", () => {
  updateViewLink();
  setDirty(true);
});

[fields.date, fields.tags, fields.summary, fields.draft, fields.toc].forEach((field) => {
  field.addEventListener("input", () => setDirty(true));
  field.addEventListener("change", () => setDirty(true));
});

fields.date.addEventListener("input", updateViewLink);
fields.date.addEventListener("change", updateViewLink);

fields.updated.addEventListener("input", () => {
  updatedTouched = true;
  setDirty(true);
});
fields.updated.addEventListener("change", () => {
  updatedTouched = true;
  setDirty(true);
});

editor.addEventListener("input", afterEdit);

editor.addEventListener("paste", async (event) => {
  const items = event.clipboardData && Array.from(event.clipboardData.items || []);
  const imageItem = items.find((item) => item.type.startsWith("image/"));
  if (!imageItem) return;
  event.preventDefault();
  try {
    const markdown = await uploadFile(imageItem.getAsFile(), "/admin/api/media/paste");
    insertAtCursor(editor, markdown);
  } catch (error) {
    setStatus(error.message);
  }
});

uploadForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const file = fileInput.files && fileInput.files[0];
  if (!file) return;
  try {
    insertAtCursor(editor, await uploadFile(file));
    fileInput.value = "";
  } catch (error) {
    setStatus(error.message);
  }
});

fileInput.addEventListener("change", async () => {
  const file = fileInput.files && fileInput.files[0];
  if (!file) return;
  try {
    insertAtCursor(editor, await uploadFile(file));
    fileInput.value = "";
  } catch (error) {
    setStatus(error.message);
  }
});

if (homeImageUpload && homeImageInput) {
  homeImageUpload.addEventListener("submit", async (event) => {
    event.preventDefault();
    const file = homeImageInput.files && homeImageInput.files[0];
    if (!file) return;
    const data = new FormData();
    data.append("file", file, file.name || "home-image.webp");
    setStatus(tr("editor.status.setting_home_image", "Setting home image"));
    const response = await fetch(apiURL("/admin/api/home-image"), { method: "POST", body: data });
    if (!response.ok) {
      setStatus((await response.text()).trim());
      return;
    }
    setStatus(tr("editor.status.home_image_set", "Home image set"));
    window.location.reload();
  });
}

if (clearHomeImageButton) {
  clearHomeImageButton.addEventListener("click", async () => {
    setStatus(tr("editor.status.clearing_home_image", "Clearing home image"));
    const response = await fetch(apiURL("/admin/api/home-image"), { method: "DELETE" });
    if (!response.ok) {
      setStatus((await response.text()).trim());
      return;
    }
    setStatus(tr("editor.status.home_image_cleared", "Home image cleared"));
    window.location.reload();
  });
}

document.querySelector("#newPostButton").addEventListener("click", newPost);
if (deletePostButton) {
  deletePostButton.addEventListener("click", () => {
    deleteCurrentPost().catch((error) => setStatus(error.message));
  });
}
document.querySelector("#saveDraftButton").addEventListener("click", () => savePost(true));
document.querySelector("#publishButton").addEventListener("click", () => savePost(false));

window.addEventListener("beforeunload", (event) => {
  if (!dirty) return;
  event.preventDefault();
  event.returnValue = "";
});

if (initialSlug) {
  loadPost(initialSlug).catch(() => newPost());
} else {
  newPost();
}
