const mediaUpload = document.querySelector("#mediaLibraryUpload");
const mediaFile = document.querySelector("#mediaLibraryFile");
const mediaStatus = document.querySelector("#mediaLibraryStatus");
const mediaGrid = document.querySelector("#mediaLibraryGrid");

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

function setMediaStatus(message) {
  if (mediaStatus) mediaStatus.textContent = message;
}

function mediaFormPayload(form) {
  return {
    original_name: form.elements.original_name.value.trim(),
    alt: form.elements.alt.value.trim(),
    caption: form.elements.caption.value.trim()
  };
}

async function copyText(text) {
  if (navigator.clipboard && window.isSecureContext) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const scratch = document.createElement("textarea");
  scratch.value = text;
  scratch.setAttribute("readonly", "");
  scratch.style.position = "fixed";
  scratch.style.left = "-9999px";
  document.body.appendChild(scratch);
  scratch.select();
  document.execCommand("copy");
  scratch.remove();
}

if (mediaUpload && mediaFile) {
  mediaUpload.addEventListener("submit", async (event) => {
    event.preventDefault();
    const file = mediaFile.files && mediaFile.files[0];
    if (!file) {
      setMediaStatus(tr("media.status.choose_file", "Choose a file"));
      return;
    }
    const data = new FormData();
    data.append("file", file, file.name);
    setMediaStatus(tr("media.status.uploading", "Uploading"));
    const response = await fetch(apiURL("/admin/api/media"), { method: "POST", body: data });
    if (!response.ok) {
      setMediaStatus((await response.text()).trim());
      return;
    }
    setMediaStatus(tr("media.status.uploaded", "Uploaded"));
    window.location.reload();
  });
}

if (mediaGrid) {
  mediaGrid.addEventListener("submit", async (event) => {
    const form = event.target.closest(".media-edit-form");
    if (!form) return;
    event.preventDefault();
    const id = form.dataset.id;
    setMediaStatus(tr("media.status.saving", "Saving"));
    const response = await fetch(apiURL(`/admin/api/media/${id}`), {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(mediaFormPayload(form))
    });
    if (!response.ok) {
      setMediaStatus((await response.text()).trim());
      return;
    }
    const result = await response.json();
    const item = result.item || {};
    const figure = form.closest(".media-item");
    const image = figure && figure.querySelector("img");
    const snippet = form.querySelector(".media-snippet");
    if (image) image.alt = item.alt || "";
    if (snippet) snippet.value = result.markdown || snippet.value;
    setMediaStatus(tr("media.status.saved", "Saved"));
  });

  mediaGrid.addEventListener("click", async (event) => {
    const copyButton = event.target.closest("[data-media-copy]");
    if (copyButton) {
      const form = copyButton.closest(".media-edit-form");
      const snippet = form && form.querySelector(".media-snippet");
      if (snippet) {
        await copyText(snippet.value);
        setMediaStatus(tr("media.status.copied", "Copied"));
      }
      return;
    }

    const deleteButton = event.target.closest("[data-media-delete]");
    if (!deleteButton) return;
    const form = deleteButton.closest(".media-edit-form");
    const figure = deleteButton.closest(".media-item");
    const name = form && form.elements.original_name.value.trim();
    if (!form || !window.confirm(formatMessage("media.confirm.delete", "Delete {name} from the media library?", { name: name || tr("editor.image.default_alt", "image") }))) return;
    setMediaStatus(tr("media.status.deleting", "Deleting"));
    const response = await fetch(apiURL(`/admin/api/media/${form.dataset.id}`), { method: "DELETE" });
    if (!response.ok) {
      setMediaStatus((await response.text()).trim());
      return;
    }
    if (figure) figure.remove();
    setMediaStatus(tr("media.status.deleted", "Deleted"));
  });
}
