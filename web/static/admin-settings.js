const resourcePackSettingsForm = document.querySelector("#resourcePackSettingsForm");
const packSettingsStatuses = Array.from(document.querySelectorAll(".pack-settings-status"));
const applyPacksButtons = Array.from(document.querySelectorAll(".apply-packs-button"));
const restoreDefaultPacksButtons = Array.from(document.querySelectorAll(".restore-default-packs-button"));
const resourcePackUploadForm = document.querySelector("#resourcePackUploadForm");
const resourcePackFile = document.querySelector("#resourcePackFile");
const localResourcePackList = document.querySelector("#localResourcePackList");
const themeLocaleGrid = document.querySelector("#themeLocaleGrid");
const pluginPackGrid = document.querySelector("#pluginPackGrid");
const pluginQueueList = document.querySelector("#pluginQueueList");
const pluginTransferAddButton = document.querySelector("#pluginTransferAdd");
const pluginTransferRemoveButton = document.querySelector("#pluginTransferRemove");
const homeImageStatus = document.querySelector("#homeImageStatus");
const homeImageUpload = document.querySelector("#homeImageUpload");
const homeImageInput = document.querySelector("#homeImageInput");
const selectHomeImageButton = document.querySelector("#selectHomeImageButton");
const clearHomeImageButton = document.querySelector("#clearHomeImageButton");

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

function setPackSettingsStatus(message) {
  packSettingsStatuses.forEach((node) => {
    node.textContent = message || "";
  });
}

function setHomeImageStatus(message) {
  if (homeImageStatus) homeImageStatus.textContent = message;
}

function selectedValue(name) {
  const selected = document.querySelector(`input[name="${name}"]:checked`);
  return selected ? String(selected.value || "") : "";
}

function currentThemePackID() {
  return selectedValue("theme_pack_id") || committedAppearance.themePackID || "";
}

function currentThemeLocale() {
  return selectedValue("theme_locale") || committedAppearance.themeLocale || "en";
}

function buildAppearancePayload(themePackID = currentThemePackID(), themeLocale = currentThemeLocale(), order = pluginOrder.slice()) {
  return {
    theme_pack: {
      pack_id: themePackID
    },
    theme_locale: themeLocale,
    plugin_order: order
  };
}

async function submitAppearancePayload(payload) {
  setPackSettingsStatus(tr("settings.status.applying", "Applying"));
  const response = await fetch(apiURL("/admin/api/resource-packs/apply"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload)
  });
  if (!response.ok) {
    setPackSettingsStatus((await response.text()).trim());
    return false;
  }
  setPackSettingsStatus(tr("settings.status.saved", "Saved"));
  window.location.reload();
  return true;
}

function syncSelectableCards(selector, selectedClass = "is-selected") {
  document.querySelectorAll(selector).forEach((card) => {
    const input = card.querySelector('input[type="radio"]');
    card.classList.toggle(selectedClass, Boolean(input && input.checked));
  });
}

function activateSelectableCard(card) {
  if (!card) return;
  const input = card.querySelector('input[type="radio"]');
  if (!input || input.checked) return;
  input.checked = true;
  input.dispatchEvent(new Event("change", { bubbles: true }));
}

function localeLabel(code) {
  switch (String(code || "").trim()) {
    case "en":
      return "English";
    case "zh-CN":
      return "简体中文";
    case "zh-TW":
      return "繁體中文";
    case "ja":
      return "日本語";
    case "ko":
      return "한국어";
    default:
      return String(code || "").trim() || "Unknown";
  }
}

function selectedThemeInput() {
  return document.querySelector('input[name="theme_pack_id"]:checked');
}

function parseThemeLocales(input) {
  if (!input) return [];
  try {
    const locales = JSON.parse(input.dataset.locales || "[]");
    return Array.isArray(locales) ? locales.map((value) => String(value || "").trim()).filter(Boolean) : [];
  } catch (_) {
    return [];
  }
}

function renderThemeLocales() {
  if (!themeLocaleGrid) return;
  const input = selectedThemeInput();
  const locales = parseThemeLocales(input);
  const defaultLocale = String((input && input.dataset.defaultLocale) || "en");
  let currentLocale = selectedValue("theme_locale");

  if (!locales.includes(currentLocale)) {
    currentLocale = locales.includes(defaultLocale) ? defaultLocale : (locales[0] || defaultLocale);
  }

  themeLocaleGrid.innerHTML = "";
  locales.forEach((code) => {
    const card = document.createElement("label");
    card.className = `locale-card${code === currentLocale ? " is-selected" : ""}`;
    card.tabIndex = 0;

    const radio = document.createElement("input");
    radio.className = "locale-card__radio";
    radio.type = "radio";
    radio.name = "theme_locale";
    radio.value = code;
    radio.checked = code === currentLocale;
    radio.setAttribute("aria-hidden", "true");

    const content = document.createElement("span");
    content.className = "locale-card__content";
    const label = document.createElement("strong");
    label.textContent = localeLabel(code);
    const meta = document.createElement("span");
    meta.className = "locale-card__meta";
    meta.textContent = code;
    content.append(label, meta);
    card.append(radio, content);

    card.addEventListener("click", () => activateSelectableCard(card));
    card.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      event.preventDefault();
      activateSelectableCard(card);
    });
    radio.addEventListener("change", () => syncSelectableCards(".locale-card"));
    themeLocaleGrid.appendChild(card);
  });
  syncSelectableCards(".locale-card");
}

document.querySelectorAll(".pack-card").forEach((card) => {
  if (card.classList.contains("plugin-card")) return;
  card.addEventListener("click", () => activateSelectableCard(card));
  card.addEventListener("keydown", (event) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    activateSelectableCard(card);
  });
});

document.querySelectorAll(".locale-card").forEach((card) => {
  card.addEventListener("click", () => activateSelectableCard(card));
  card.addEventListener("keydown", (event) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    activateSelectableCard(card);
  });
});

document.querySelectorAll('.pack-card input[type="radio"]').forEach((input) => {
  input.addEventListener("change", () => {
    syncSelectableCards(".pack-card:not(.plugin-card)");
    renderThemeLocales();
  });
});
syncSelectableCards(".pack-card:not(.plugin-card)");
renderThemeLocales();

const pluginCards = Array.from(pluginPackGrid ? pluginPackGrid.querySelectorAll(".plugin-card") : []);
pluginCards.forEach((card, index) => {
  card.dataset.pluginIndex = String(index);
});

const pluginRegistry = {};
document.querySelectorAll("[data-plugin-id][data-plugin-name]").forEach((node) => {
  const pluginID = node.dataset.pluginId;
  if (!pluginID) return;
  pluginRegistry[pluginID] = {
    id: pluginID,
    name: node.dataset.pluginName || pluginID,
    description: node.dataset.pluginDescription || ""
  };
});

function parseJSONDataset(value, fallback) {
  try {
    const parsed = JSON.parse(value);
    return parsed == null ? fallback : parsed;
  } catch (_) {
    return fallback;
  }
}

const committedAppearance = {
  themePackID: String((resourcePackSettingsForm && resourcePackSettingsForm.dataset.currentThemePackId) || ""),
  themeLocale: String((resourcePackSettingsForm && resourcePackSettingsForm.dataset.currentThemeLocale) || "en"),
  pluginOrder: parseJSONDataset((resourcePackSettingsForm && resourcePackSettingsForm.dataset.currentPluginOrder) || "[]", [])
};

let pluginOrder = committedAppearance.pluginOrder.slice();

let selectedCatalogPluginID = pluginCards[0] ? pluginCards[0].dataset.pluginId : "";
let selectedQueuePluginID = pluginOrder[0] || "";

function renderPluginCards() {
  if (!pluginPackGrid) return;

  pluginPackGrid.innerHTML = "";
  const availablePluginIDs = pluginCards
    .map((card) => card.dataset.pluginId)
    .filter((pluginID) => !pluginOrder.includes(pluginID));

  if (!availablePluginIDs.includes(selectedCatalogPluginID)) {
    selectedCatalogPluginID = availablePluginIDs[0] || "";
  }

  pluginCards.forEach((card) => {
    const pluginID = card.dataset.pluginId;
    const activeIndex = pluginOrder.indexOf(pluginID);
    const active = activeIndex >= 0;

    card.classList.toggle("is-active", active);
    card.classList.toggle("is-selected", !active && pluginID === selectedCatalogPluginID);
    card.dataset.pluginActive = active ? "1" : "0";
    card.dataset.pluginOrder = active ? String(activeIndex + 1) : "0";

    const badge = card.querySelector(".plugin-order-badge");
    const addButton = card.querySelector("[data-plugin-add]");

    if (badge) badge.textContent = active ? `#${activeIndex + 1}` : "";
    if (addButton) addButton.hidden = active;

    if (!active) {
      pluginPackGrid.appendChild(card);
    }
  });

  renderPluginQueue();
  updatePluginControls();
}

function renderPluginQueue() {
  if (!pluginQueueList) return;

  pluginQueueList.innerHTML = "";
  if (selectedQueuePluginID && !pluginOrder.includes(selectedQueuePluginID)) {
    selectedQueuePluginID = pluginOrder[0] || "";
  }
  if (!pluginOrder.length) {
    return;
  }

  pluginOrder.forEach((pluginID, index) => {
    const plugin = pluginRegistry[pluginID];
    if (!plugin) return;

    const item = document.createElement("article");
    item.className = `pack-card plugin-card plugin-catalog-card plugin-queue-item${pluginID === selectedQueuePluginID ? " is-selected" : ""}`;
    item.dataset.pluginId = pluginID;
    item.dataset.pluginName = plugin.name;
    item.dataset.pluginDescription = plugin.description;

    const body = document.createElement("div");
    body.className = "plugin-queue-item__body";
    const copy = document.createElement("div");
    copy.className = "plugin-queue-item__copy";
    const main = document.createElement("div");
    main.className = "plugin-queue-item__main";
    const text = document.createElement("div");
    text.className = "plugin-queue-item__text";
    const name = document.createElement("strong");
    name.textContent = plugin.name;
    const description = document.createElement("p");
    description.className = "plugin-queue-item__description";
    description.textContent = plugin.description;

    text.appendChild(name);
    main.appendChild(text);
    copy.append(main, description);

    const sortControls = document.createElement("div");
    sortControls.className = "plugin-sort-controls";
    sortControls.setAttribute("aria-label", "Plugin ordering");
    const sortIndex = document.createElement("div");
    sortIndex.className = "plugin-sort-index";
    sortIndex.textContent = `#${index + 1}`;
    sortControls.append(
      sortIndex,
      pluginArrowButton("up", index <= 0),
      pluginArrowButton("down", index >= pluginOrder.length - 1)
    );

    body.append(copy, sortControls);
    item.appendChild(body);
    pluginQueueList.appendChild(item);
  });
}

function updatePluginControls() {
  const queueIndex = pluginOrder.indexOf(selectedQueuePluginID);
  if (pluginTransferAddButton) {
    pluginTransferAddButton.disabled = !selectedCatalogPluginID || pluginOrder.includes(selectedCatalogPluginID);
  }
  if (pluginTransferRemoveButton) {
    pluginTransferRemoveButton.disabled = queueIndex < 0;
  }
}

function addPlugin(pluginID) {
  if (!pluginID || pluginOrder.includes(pluginID)) return;
  pluginOrder.push(pluginID);
  selectedQueuePluginID = pluginID;
  renderPluginCards();
}

function removePlugin(pluginID) {
  const index = pluginOrder.indexOf(pluginID);
  pluginOrder = pluginOrder.filter((id) => id !== pluginID);
  if (selectedQueuePluginID === pluginID) {
    selectedQueuePluginID = pluginOrder[index] || pluginOrder[index - 1] || "";
  }
  renderPluginCards();
}

function movePlugin(pluginID, direction) {
  const index = pluginOrder.indexOf(pluginID);
  if (index < 0) return;
  const nextIndex = direction === "up" ? index - 1 : index + 1;
  if (nextIndex < 0 || nextIndex >= pluginOrder.length) return;
  const next = pluginOrder.slice();
  [next[index], next[nextIndex]] = [next[nextIndex], next[index]];
  pluginOrder = next;
  selectedQueuePluginID = pluginID;
  renderPluginCards();
}

function pluginArrowButton(direction, disabled) {
  const action = direction === "up" ? "move_up" : "move_down";
  const label = tr(`settings.plugins.${action}`, direction === "up" ? "Move Up" : "Move Down");
  const button = document.createElement("button");
  button.type = "button";
  button.className = `ui-button plugin-arrow-button plugin-arrow-button--${direction}`;
  button.title = label;
  button.setAttribute("aria-label", label);
  button.disabled = disabled;
  button.setAttribute(direction === "up" ? "data-plugin-up" : "data-plugin-down", "");
  return button;
}

if (pluginPackGrid) {
  pluginPackGrid.addEventListener("click", (event) => {
    const card = event.target.closest(".plugin-card");
    if (!card) return;
    const pluginID = card.dataset.pluginId;
    selectedCatalogPluginID = pluginID;
    renderPluginCards();
  });
  renderPluginCards();
}

if (pluginQueueList) {
  pluginQueueList.addEventListener("click", (event) => {
    const item = event.target.closest(".plugin-queue-item");
    if (!item) return;
    const pluginID = item.dataset.pluginId;
    if (event.target.closest("[data-plugin-up]")) {
      movePlugin(pluginID, "up");
      return;
    }
    if (event.target.closest("[data-plugin-down]")) {
      movePlugin(pluginID, "down");
      return;
    }
    selectedQueuePluginID = pluginID;
    renderPluginCards();
  });
}

if (pluginTransferAddButton) {
  pluginTransferAddButton.addEventListener("click", () => addPlugin(selectedCatalogPluginID));
}

if (pluginTransferRemoveButton) {
  pluginTransferRemoveButton.addEventListener("click", () => removePlugin(selectedQueuePluginID));
}

if (applyPacksButtons.length) {
  applyPacksButtons.forEach((button) => {
    button.addEventListener("click", async () => {
      const scope = button.dataset.applyScope || "all";
      if (scope === "theme") {
        await submitAppearancePayload(buildAppearancePayload(
          currentThemePackID(),
          currentThemeLocale(),
          pluginOrder.slice()
        ));
        return;
      }
      if (scope === "plugin") {
        await submitAppearancePayload(buildAppearancePayload(
          currentThemePackID(),
          currentThemeLocale(),
          pluginOrder.slice()
        ));
        return;
      }
      await submitAppearancePayload(buildAppearancePayload());
    });
  });
}

if (restoreDefaultPacksButtons.length && resourcePackSettingsForm) {
  restoreDefaultPacksButtons.forEach((button) => {
    button.addEventListener("click", async () => {
      const scope = button.dataset.restoreScope || "all";
      const defaultThemePackID = String(resourcePackSettingsForm.dataset.defaultThemePackId || "");
      const defaultThemeLocale = String(resourcePackSettingsForm.dataset.defaultThemeLocale || "en");
      if (scope === "theme") {
        document.querySelectorAll('input[name="theme_pack_id"]').forEach((input) => {
          input.checked = input.value === defaultThemePackID;
        });
        syncSelectableCards(".pack-card:not(.plugin-card)");
        renderThemeLocales();
        document.querySelectorAll('input[name="theme_locale"]').forEach((input) => {
          input.checked = input.value === defaultThemeLocale;
        });
        syncSelectableCards(".locale-card");
        await submitAppearancePayload(buildAppearancePayload(
          defaultThemePackID,
          defaultThemeLocale,
          pluginOrder.slice()
        ));
        return;
      }

      if (scope === "plugin") {
        pluginOrder = [];
        renderPluginCards();
        await submitAppearancePayload(buildAppearancePayload(
          currentThemePackID(),
          currentThemeLocale(),
          []
        ));
        return;
      }

      document.querySelectorAll('input[name="theme_pack_id"]').forEach((input) => {
        input.checked = input.value === defaultThemePackID;
      });
      syncSelectableCards(".pack-card:not(.plugin-card)");
      renderThemeLocales();
      document.querySelectorAll('input[name="theme_locale"]').forEach((input) => {
        input.checked = input.value === defaultThemeLocale;
      });
      pluginOrder = [];
      syncSelectableCards(".locale-card");
      renderPluginCards();
      await submitAppearancePayload(buildAppearancePayload(defaultThemePackID, defaultThemeLocale, []));
    });
  });
}

if (resourcePackUploadForm && resourcePackFile) {
  resourcePackUploadForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const file = resourcePackFile.files && resourcePackFile.files[0];
    if (!file) {
      setPackSettingsStatus(tr("settings.upload_pack.empty", "Choose a .zip package"));
      return;
    }
    const data = new FormData();
    data.append("file", file, file.name || "resource-pack.zip");
    setPackSettingsStatus(tr("settings.status.uploading", "Uploading"));
    const response = await fetch(apiURL("/admin/api/resource-packs"), { method: "POST", body: data });
    if (!response.ok) {
      setPackSettingsStatus((await response.text()).trim());
      return;
    }
    setPackSettingsStatus(tr("settings.status.installed", "Installed"));
    window.location.reload();
  });
}

if (localResourcePackList) {
  localResourcePackList.addEventListener("click", async (event) => {
    const button = event.target.closest("[data-resource-pack-delete]");
    if (!button) return;

    const packType = String(button.dataset.packType || "").trim();
    const packID = String(button.dataset.packId || "").trim();
    const packName = String(button.dataset.packName || packID).trim();
    const active = button.dataset.packActive === "1";
    if (!packType || !packID) return;

    const confirmKey = active ? "settings.local_packs.confirm_delete_active" : "settings.local_packs.confirm_delete";
    const confirmFallback = active
      ? "Delete {name}? It is currently in use, so the default appearance will be restored."
      : "Delete {name}?";
    if (!window.confirm(formatMessage(confirmKey, confirmFallback, { name: packName || packID }))) return;

    button.disabled = true;
    setPackSettingsStatus(tr("settings.status.deleting", "Deleting"));
    const response = await fetch(apiURL(`/admin/api/resource-packs/${encodeURIComponent(packType)}/${encodeURIComponent(packID)}`), {
      method: "DELETE"
    });
    if (!response.ok) {
      button.disabled = false;
      setPackSettingsStatus((await response.text()).trim());
      return;
    }
    setPackSettingsStatus(tr("settings.status.deleted", "Deleted"));
    window.location.reload();
  });
}

if (homeImageUpload && homeImageInput) {
  homeImageUpload.addEventListener("submit", async (event) => {
    event.preventDefault();
    const file = homeImageInput.files && homeImageInput.files[0];
    if (!file) return;
    const data = new FormData();
    data.append("file", file, file.name || "home-image.webp");
    setHomeImageStatus(tr("settings.status.uploading", "Uploading"));
    const response = await fetch(apiURL("/admin/api/home-image"), { method: "POST", body: data });
    if (!response.ok) {
      setHomeImageStatus((await response.text()).trim());
      return;
    }
    setHomeImageStatus(tr("settings.status.saved", "Saved"));
    window.location.reload();
  });
}

if (selectHomeImageButton) {
  selectHomeImageButton.addEventListener("click", async () => {
    if (!window.PostizerMediaPicker) {
      setHomeImageStatus(tr("media_picker.error", "Media selection failed"));
      return;
    }
    await window.PostizerMediaPicker.open({
      title: tr("settings.home_image.choose_media", "Choose from media library"),
      selectLabel: tr("media_picker.use_as_home_image", "Use as Home Image"),
      currentPath: selectHomeImageButton.dataset.currentHomeImage || "",
      onSelect: async (item) => {
        setHomeImageStatus(tr("settings.status.applying", "Applying"));
        const response = await fetch(apiURL("/admin/api/home-image"), {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ media_id: item.id })
        });
        if (!response.ok) {
          throw new Error((await response.text()).trim());
        }
        setHomeImageStatus(tr("settings.status.saved", "Saved"));
        window.location.reload();
      }
    });
  });
}

if (clearHomeImageButton) {
  clearHomeImageButton.addEventListener("click", async () => {
    setHomeImageStatus(tr("settings.status.clearing", "Clearing"));
    const response = await fetch(apiURL("/admin/api/home-image"), { method: "DELETE" });
    if (!response.ok) {
      setHomeImageStatus((await response.text()).trim());
      return;
    }
    setHomeImageStatus(tr("settings.status.cleared", "Cleared"));
    window.location.reload();
  });
}
