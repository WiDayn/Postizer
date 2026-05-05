function postizerMediaPickerMessage(key, fallback) {
  if (window.postizerMessage) return window.postizerMessage(key, fallback);
  return fallback;
}

function postizerMediaPickerAPIURL(path) {
  const token = new URLSearchParams(window.location.search).get("token");
  if (!token) return path;
  const url = new URL(path, window.location.origin);
  url.searchParams.set("token", token);
  return url.pathname + url.search;
}

function mediaPickerLabel(item) {
  return item.original_name || item.alt || item.caption || item.id || "media";
}

function mediaPickerMeta(item) {
  const dimensions = item.width && item.height ? `${item.width}x${item.height}` : "";
  return [dimensions, item.mime_type].filter(Boolean).join(" / ");
}

function closeMediaPicker(modal, previousFocus) {
  modal.remove();
  document.body.classList.remove("has-open-modal");
  if (previousFocus && typeof previousFocus.focus === "function") previousFocus.focus();
}

async function openMediaPicker(options = {}) {
  const previousFocus = document.activeElement;
  const modal = document.createElement("div");
  modal.className = "media-picker-modal";
  modal.setAttribute("role", "dialog");
  modal.setAttribute("aria-modal", "true");

  const panel = document.createElement("section");
  panel.className = "media-picker-panel";
  panel.setAttribute("aria-labelledby", "mediaPickerTitle");

  const header = document.createElement("div");
  header.className = "media-picker-header";

  const title = document.createElement("h2");
  title.id = "mediaPickerTitle";
  title.textContent = options.title || postizerMediaPickerMessage("media_picker.title", "Media Library");

  const closeButton = document.createElement("button");
  closeButton.type = "button";
  closeButton.className = "ui-button ui-button--ghost";
  closeButton.textContent = postizerMediaPickerMessage("media_picker.close", "Close");

  header.append(title, closeButton);

  const status = document.createElement("p");
  status.className = "media-picker-status";
  status.textContent = postizerMediaPickerMessage("media_picker.loading", "Loading media");

  const grid = document.createElement("div");
  grid.className = "media-picker-grid";

  const actions = document.createElement("div");
  actions.className = "media-picker-actions";

  const selectedSummary = document.createElement("span");
  selectedSummary.className = "media-picker-selected";
  selectedSummary.textContent = postizerMediaPickerMessage("media_picker.none_selected", "No media selected");

  const cancelButton = document.createElement("button");
  cancelButton.type = "button";
  cancelButton.className = "ui-button ui-button--ghost";
  cancelButton.textContent = postizerMediaPickerMessage("media_picker.cancel", "Cancel");

  const selectButton = document.createElement("button");
  selectButton.type = "button";
  selectButton.className = "ui-button ui-button--primary";
  selectButton.textContent = options.selectLabel || postizerMediaPickerMessage("media_picker.select", "Use Selected");
  selectButton.disabled = true;

  actions.append(selectedSummary, cancelButton, selectButton);
  panel.append(header, status, grid, actions);
  modal.appendChild(panel);
  document.body.appendChild(modal);
  document.body.classList.add("has-open-modal");
  closeButton.focus();

  let keyHandler = null;
  const close = () => {
    if (keyHandler) document.removeEventListener("keydown", keyHandler);
    closeMediaPicker(modal, previousFocus);
  };

  closeButton.addEventListener("click", close);
  cancelButton.addEventListener("click", close);

  let selectedItem = null;
  const setSelected = (item, button) => {
    selectedItem = item;
    grid.querySelectorAll(".media-picker-item").forEach((node) => {
      node.classList.toggle("is-selected", node === button);
      node.setAttribute("aria-pressed", node === button ? "true" : "false");
    });
    selectedSummary.textContent = `${postizerMediaPickerMessage("media_picker.selected_prefix", "Selected")}: ${mediaPickerLabel(item)}`;
    selectButton.disabled = false;
  };

  selectButton.addEventListener("click", async () => {
    if (!selectedItem) return;
    selectButton.disabled = true;
    status.textContent = postizerMediaPickerMessage("media_picker.applying", "Applying");
    try {
      if (typeof options.onSelect === "function") {
        await options.onSelect(selectedItem);
      }
      close();
    } catch (error) {
      status.textContent = error && error.message ? error.message : postizerMediaPickerMessage("media_picker.error", "Media selection failed");
      selectButton.disabled = false;
    }
  });

  modal.addEventListener("click", (event) => {
    if (event.target === modal) close();
  });

  keyHandler = (event) => {
    if (event.key === "Escape" && document.body.contains(modal)) {
      close();
    }
  };
  document.addEventListener("keydown", keyHandler);

  try {
    const response = await fetch(postizerMediaPickerAPIURL(options.endpoint || "/admin/api/media"));
    if (!response.ok) {
      status.textContent = (await response.text()).trim();
      return;
    }
    let items = await response.json();
    if (!Array.isArray(items)) items = [];
    if (options.imagesOnly !== false) {
      items = items.filter((item) => String(item.mime_type || "").startsWith("image/"));
    }

    grid.innerHTML = "";
    if (!items.length) {
      status.textContent = postizerMediaPickerMessage("media_picker.empty", "No media available");
      return;
    }
    status.textContent = "";

    items.forEach((item) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "media-picker-item";
      button.setAttribute("aria-pressed", "false");

      const mark = document.createElement("span");
      mark.className = "media-picker-item__mark";
      mark.textContent = postizerMediaPickerMessage("media_picker.selected_prefix", "Selected");

      const image = document.createElement("img");
      image.src = item.path || "";
      image.alt = item.alt || "";

      const text = document.createElement("span");
      text.className = "media-picker-item__text";

      const name = document.createElement("strong");
      name.textContent = mediaPickerLabel(item);

      const meta = document.createElement("span");
      meta.textContent = mediaPickerMeta(item);

      text.append(name, meta);
      button.append(mark, image, text);
      button.addEventListener("click", () => setSelected(item, button));
      grid.appendChild(button);

      if (options.currentPath && item.path === options.currentPath) {
        setSelected(item, button);
      }
    });
  } catch (error) {
    status.textContent = error && error.message ? error.message : postizerMediaPickerMessage("media_picker.error", "Media selection failed");
  }
}

window.PostizerMediaPicker = {
  open: openMediaPicker
};
