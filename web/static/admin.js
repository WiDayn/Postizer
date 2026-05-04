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
const mediaStrip = document.querySelector("#mediaStrip");
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
let slugTouched = false;
let previewTimer = 0;
let dirty = false;
const postsCollapsedKey = "postizer.editor.postsCollapsed";

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
  postLibraryToggle.title = collapsed ? tr("editor.library.expand", "Expand posts") : tr("editor.library.collapse", "Collapse posts");
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

function today() {
  return new Date().toISOString().slice(0, 10);
}

function slugify(value) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .replace(/-+/g, "-");
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
  const base = slugify(fields.slug.value || fields.title.value || currentSlug || "figure") || "figure";
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
  return {
    title: fields.title.value.trim(),
    slug: slugify(fields.slug.value || fields.title.value),
    date: fields.date.value || today(),
    updated: fields.updated.value,
    tags: fields.tags.value.split(",").map((tag) => tag.trim()).filter(Boolean),
    summary: fields.summary.value.trim(),
    draft: typeof draftOverride === "boolean" ? draftOverride : fields.draft.checked,
    toc: fields.toc.checked,
    body: editor.value
  };
}

function fillForm(post) {
  currentSlug = post.slug || "";
  slugTouched = Boolean(post.slug);
  fields.title.value = post.title || "";
  fields.slug.value = post.slug || "";
  fields.date.value = post.date || today();
  fields.updated.value = post.updated || "";
  fields.tags.value = (post.tags || []).join(", ");
  fields.summary.value = post.summary || "";
  fields.draft.checked = Boolean(post.draft);
  fields.toc.checked = post.toc !== false;
  editor.value = post.body || "";
  updateViewLink();
  updateCounts();
  schedulePreview(true);
  setDirty(false);
}

async function loadPost(slug) {
  setStatus(tr("editor.status.loading", "Loading"));
  const response = await fetch(apiURL(`/admin/api/posts/${slug}`));
  if (!response.ok) throw new Error(await response.text());
  fillForm(await response.json());
  postLibrary.querySelectorAll("button").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.slug === slug);
  });
  setStatus(tr("editor.status.ready", "Ready"));
}

function newPost() {
  fillForm({
    title: "",
    slug: "",
    date: today(),
    updated: "",
    tags: [],
    summary: "",
    draft: true,
    toc: true,
    body: "## Notes\n\n"
  });
  currentSlug = "";
  slugTouched = false;
  postLibrary.querySelectorAll("button").forEach((button) => button.classList.remove("is-active"));
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
  const response = await fetch(apiURL("/admin/api/posts"), {
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
  fields.slug.value = result.slug;
  fields.draft.checked = Boolean(result.draft);
  updateViewLink();
  await refreshPostLibrary(result.slug);
  setDirty(false);
  setStatus(result.draft ? tr("editor.status.draft_saved", "Draft saved") : tr("editor.status.published", "Published"));
}

async function refreshPostLibrary(activeSlug) {
  const response = await fetch(apiURL("/admin/api/posts"));
  if (!response.ok) return;
  const posts = await response.json();
  postLibrary.innerHTML = "";
  posts.forEach((post) => {
    const li = document.createElement("li");
    const button = document.createElement("button");
    button.type = "button";
    button.className = "ui-list-button";
    button.dataset.slug = post.slug;
    button.classList.toggle("is-active", post.slug === activeSlug);
    button.innerHTML = "<strong></strong><span></span>";
    button.querySelector("strong").textContent = post.title;
    button.querySelector("span").textContent = `${post.date || ""} ${post.draft ? tr("common.draft", "Draft") : tr("common.published", "Published")}`;
    li.appendChild(button);
    postLibrary.appendChild(li);
  });
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
  const matches = editor.value.match(/[\p{L}\p{N}_]+/gu);
  if (wordCount) wordCount.textContent = formatMessage("editor.word_count", "{count} words", { count: matches ? matches.length : 0 });
}

function updateViewLink() {
  const slug = slugify(fields.slug.value || fields.title.value || currentSlug);
  fields.view.href = slug ? `/posts/${slug}` : "/";
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
  setStatus(tr("editor.status.uploaded", "Uploaded"));
  await refreshMedia();
  return figureMarkdown(result.item || {});
}

async function refreshMedia() {
  const response = await fetch(apiURL("/admin/api/media"));
  if (!response.ok) return;
  const items = await response.json();
  mediaStrip.innerHTML = "";
  items.slice(0, 18).forEach((item) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "ui-media-button";
    button.dataset.path = item.path;
    button.dataset.alt = item.alt || tr("editor.image.default_alt", "Image");
    button.dataset.caption = item.caption || "";
    button.title = item.original_name;
    const img = document.createElement("img");
    img.src = item.path;
    img.alt = item.alt || "";
    button.appendChild(img);
    mediaStrip.appendChild(button);
  });
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

postLibrary.addEventListener("click", (event) => {
  const button = event.target.closest("button[data-slug]");
  if (!button) return;
  loadPost(button.dataset.slug).catch((error) => setStatus(error.message));
});

mediaStrip.addEventListener("click", (event) => {
  const button = event.target.closest("button[data-path]");
  if (button) {
    insertAtCursor(editor, figureMarkdown({
      path: button.dataset.path,
      alt: button.dataset.alt,
      caption: button.dataset.caption
    }));
  }
});

fields.title.addEventListener("input", () => {
  if (!slugTouched) fields.slug.value = slugify(fields.title.value);
  updateViewLink();
  setDirty(true);
});

fields.slug.addEventListener("input", () => {
  slugTouched = true;
  fields.slug.value = slugify(fields.slug.value);
  updateViewLink();
  setDirty(true);
});

[fields.date, fields.updated, fields.tags, fields.summary, fields.draft, fields.toc].forEach((field) => {
  field.addEventListener("input", () => setDirty(true));
  field.addEventListener("change", () => setDirty(true));
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
document.querySelector("#saveDraftButton").addEventListener("click", () => savePost(true));
document.querySelector("#publishButton").addEventListener("click", () => savePost(false));

window.addEventListener("beforeunload", (event) => {
  if (!dirty) return;
  event.preventDefault();
  event.returnValue = "";
});

const firstPost = postLibrary.querySelector("button[data-slug]");
if (firstPost) {
  loadPost(firstPost.dataset.slug).catch(() => newPost());
} else {
  newPost();
}
