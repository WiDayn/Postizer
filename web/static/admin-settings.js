const resourcePackSettingsForm = document.querySelector("#resourcePackSettingsForm");
const packSettingsStatuses = Array.from(document.querySelectorAll(".pack-settings-status"));
const applyPacksButtons = Array.from(document.querySelectorAll(".apply-packs-button"));
const cancelChangesButtons = Array.from(document.querySelectorAll(".cancel-changes-button"));
const restoreDefaultPacksButtons = Array.from(document.querySelectorAll(".restore-default-packs-button"));
const resourcePackUploadForm = document.querySelector("#resourcePackUploadForm");
const resourcePackFile = document.querySelector("#resourcePackFile");
const localResourcePackList = document.querySelector("#localResourcePackList");
const resourceMarketplaceList = document.querySelector("#resourceMarketplaceList");
const resourceMarketplaceStatus = document.querySelector("#resourceMarketplaceStatus");
const resourceMarketplaceRefresh = document.querySelector("#resourceMarketplaceRefresh");
const resourceMarketplaceFilterButtons = Array.from(document.querySelectorAll("[data-resource-marketplace-filter]"));
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
const siteTitleSettingsForm = document.querySelector("#siteTitleSettingsForm");
const siteTitleStatus = document.querySelector("#siteTitleStatus");
const siteTitleMain = document.querySelector("#siteTitleMain");
const siteTitleSubtitle = document.querySelector("#siteTitleSubtitle");
const adminAccountSettingsForm = document.querySelector("#adminAccountSettingsForm");
const adminAccountStatus = document.querySelector("#adminAccountStatus");
const adminAccountUsername = document.querySelector("#adminAccountUsername");
const adminAccountCurrentPassword = document.querySelector("#adminAccountCurrentPassword");
const adminAccountNewPassword = document.querySelector("#adminAccountNewPassword");
const adminAccountConfirmPassword = document.querySelector("#adminAccountConfirmPassword");
const permalinkSettingsForm = document.querySelector("#permalinkSettingsForm");
const permalinkStatus = document.querySelector("#permalinkStatus");
const permalinkPostPattern = document.querySelector("#permalinkPostPattern");
const permalinkPagePattern = document.querySelector("#permalinkPagePattern");
const permalinkTagPattern = document.querySelector("#permalinkTagPattern");
const permalinkPostPreview = document.querySelector("#permalinkPostPreview");
const permalinkPagePreview = document.querySelector("#permalinkPagePreview");
const permalinkTagPreview = document.querySelector("#permalinkTagPreview");
const permalinkTokenButtons = Array.from(document.querySelectorAll("[data-permalink-token]"));
const permalinkPostPresetInputs = Array.from(document.querySelectorAll('input[name="post_permalink_preset"]'));
const autoUpdateSettingsForm = document.querySelector("#autoUpdateSettingsForm");
const autoUpdateStatus = document.querySelector("#autoUpdateStatus");
const autoUpdateEnabled = document.querySelector("#autoUpdateEnabled");
const commentSettingsForm = document.querySelector("#commentSettingsForm");
const commentSettingsStatus = document.querySelector("#commentSettingsStatus");
const commentsEnabled = document.querySelector("#commentsEnabled");
const homePageSettingsForm = document.querySelector("#homePageSettingsForm");
const homePageSettingsStatus = document.querySelector("#homePageSettingsStatus");
const homePagePageSize = document.querySelector("#homePagePageSize");
const timeZoneSettingsForm = document.querySelector("#timeZoneSettingsForm");
const timeZoneStatus = document.querySelector("#timeZoneStatus");
const siteTimeZone = document.querySelector("#siteTimeZone");
const mediaProcessingSettingsForm = document.querySelector("#mediaProcessingSettingsForm");
const mediaProcessingStatus = document.querySelector("#mediaProcessingStatus");
const mediaAutoWebP = document.querySelector("#mediaAutoWebP");
const mediaWebPQuality = document.querySelector("#mediaWebPQuality");
const mediaWebPQualityRange = document.querySelector("#mediaWebPQualityRange");
const mediaKeepOriginal = document.querySelector("#mediaKeepOriginal");

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

function setResourceMarketplaceStatus(message) {
  if (resourceMarketplaceStatus) resourceMarketplaceStatus.textContent = message || "";
}

function setHomeImageStatus(message) {
  if (homeImageStatus) homeImageStatus.textContent = message;
}

function setSiteTitleStatus(message) {
  if (siteTitleStatus) siteTitleStatus.textContent = message;
}

function setAdminAccountStatus(message) {
  if (adminAccountStatus) adminAccountStatus.textContent = message;
}

function setPermalinkStatus(message) {
  if (permalinkStatus) permalinkStatus.textContent = message;
}

function setAutoUpdateStatus(message) {
  if (autoUpdateStatus) autoUpdateStatus.textContent = message;
}

function setCommentSettingsStatus(message) {
  if (commentSettingsStatus) commentSettingsStatus.textContent = message;
}

function setHomePageSettingsStatus(message) {
  if (homePageSettingsStatus) homePageSettingsStatus.textContent = message;
}

function setTimeZoneStatus(message) {
  if (timeZoneStatus) timeZoneStatus.textContent = message;
}

function setMediaProcessingStatus(message) {
  if (mediaProcessingStatus) mediaProcessingStatus.textContent = message;
}

function normalizedQuality(value) {
  const parsed = Number.parseInt(String(value || ""), 10);
  if (!Number.isFinite(parsed)) return 82;
  return Math.min(100, Math.max(1, parsed));
}

function normalizedHomePageSize(value) {
  const parsed = Number.parseInt(String(value || ""), 10);
  if (!Number.isFinite(parsed)) return 10;
  return Math.min(100, Math.max(1, parsed));
}

function formatPermalinkPreview(pattern, values) {
  let path = String(pattern || "").replace(/%([A-Za-z0-9_]+)%/g, (token, name) => {
    const key = `%${String(name || "").toLowerCase()}%`;
    return Object.prototype.hasOwnProperty.call(values, key) ? values[key] : token;
  });
  if (!path.startsWith("/")) path = `/${path}`;
  return path;
}

function syncPermalinkPreviews() {
  if (permalinkPostPreview && permalinkPostPattern) {
    permalinkPostPreview.textContent = formatPermalinkPreview(permalinkPostPattern.value || "/posts/%postname%", {
      "%postname%": "sample-post",
      "%slug%": "sample-post",
      "%year%": "2026",
      "%monthnum%": "05",
      "%day%": "20"
    });
  }
  if (permalinkPagePreview && permalinkPagePattern) {
    permalinkPagePreview.textContent = formatPermalinkPreview(permalinkPagePattern.value || "/pages/%pagename%", {
      "%pagename%": "about",
      "%slug%": "about"
    });
  }
  if (permalinkTagPreview && permalinkTagPattern) {
    permalinkTagPreview.textContent = formatPermalinkPreview(permalinkTagPattern.value || "/tags/%tag%", {
      "%tag%": "go",
      "%slug%": "go"
    });
  }
}

function syncPostPermalinkPreset() {
  if (!permalinkPostPresetInputs.length || !permalinkPostPattern) return;
  const value = String(permalinkPostPattern.value || "").trim();
  const matched = permalinkPostPresetInputs.find((input) => input.value !== "custom" && input.value === value);
  permalinkPostPresetInputs.forEach((input) => {
    input.checked = matched ? input === matched : input.value === "custom";
    if (input.value === "custom") {
      const preview = input.closest(".permalink-preset") && input.closest(".permalink-preset").querySelector("code");
      if (preview) preview.textContent = value || "/posts/%postname%";
    }
  });
}

function insertPermalinkToken(input, token) {
  if (!input) return;
  const start = input.selectionStart || input.value.length;
  const end = input.selectionEnd || start;
  input.value = `${input.value.slice(0, start)}${token}${input.value.slice(end)}`;
  input.selectionStart = input.selectionEnd = start + token.length;
  input.focus();
  syncPermalinkPreviews();
  syncPostPermalinkPreset();
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

function marketplaceButtonLabel(item) {
  if (item && item.update_available) return tr("settings.resource_marketplace.update", "Update");
  if (item && item.installed) return tr("settings.resource_marketplace.reinstall", "Reinstall");
  return tr("settings.resource_marketplace.install", "Install");
}

function marketplaceTagLabel(tag) {
  if (tag === "theme") return tr("settings.resource_marketplace.tag.theme", "Theme");
  if (tag === "plugin") return tr("settings.resource_marketplace.tag.plugin", "Plugin");
  return tag;
}

function appendMarketplaceBadge(container, text) {
  if (!text) return;
  const badge = document.createElement("span");
  badge.className = "marketplace-badge";
  badge.textContent = text;
  container.appendChild(badge);
}

function appendMarketplaceMemberList(container, title, members) {
  if (!Array.isArray(members) || !members.length) return;
  const group = document.createElement("div");
  group.className = "marketplace-card__member-group";
  const heading = document.createElement("h4");
  heading.textContent = title;
  group.appendChild(heading);
  const list = document.createElement("ul");
  members.forEach((member) => {
    const item = document.createElement("li");
    const name = document.createElement("span");
    name.textContent = member.name || member.id || "";
    item.appendChild(name);
    if (member.version) {
      const version = document.createElement("small");
      version.textContent = `v${member.version}`;
      item.appendChild(version);
    }
    list.appendChild(item);
  });
  group.appendChild(list);
  container.appendChild(group);
}

let resourceMarketplaceItems = [];
let resourceMarketplaceFilter = "all";

function filteredResourceMarketplaceItems() {
  if (resourceMarketplaceFilter === "all") return resourceMarketplaceItems.slice();
  return resourceMarketplaceItems.filter((item) => {
    const tags = Array.isArray(item.tags) ? item.tags : [];
    return tags.includes(resourceMarketplaceFilter);
  });
}

function renderResourceMarketplace() {
  if (!resourceMarketplaceList) return;
  resourceMarketplaceList.innerHTML = "";
  const items = filteredResourceMarketplaceItems();
  if (!items.length) {
    const empty = document.createElement("p");
    empty.className = "marketplace-empty";
    empty.textContent = resourceMarketplaceItems.length
      ? tr("settings.resource_marketplace.filter_empty", "No resource packs match this filter.")
      : tr("settings.resource_marketplace.empty", "No resource packs are listed yet.");
    resourceMarketplaceList.appendChild(empty);
    return;
  }

  items.forEach((item) => {
    const card = document.createElement("article");
    card.className = "marketplace-card";
    card.dataset.resourceMarketplaceId = item.id || "";

    const preview = document.createElement("div");
    preview.className = "marketplace-card__preview";
    if (item.preview) {
      const image = document.createElement("img");
      image.src = item.preview;
      image.alt = "";
      image.loading = "lazy";
      image.addEventListener("error", () => {
        preview.classList.add("is-missing");
        image.remove();
      });
      preview.appendChild(image);
    } else {
      preview.classList.add("is-missing");
    }
    card.appendChild(preview);

    const body = document.createElement("div");
    body.className = "marketplace-card__body";

    const titleRow = document.createElement("div");
    titleRow.className = "marketplace-card__title-row";
    const title = document.createElement("h3");
    title.textContent = item.name || item.id || "";
    titleRow.appendChild(title);
    if (item.release && item.release.tag) {
      const version = document.createElement("span");
      version.className = "pack-version-badge";
      version.textContent = item.release.tag;
      titleRow.appendChild(version);
    }
    body.appendChild(titleRow);

    const summary = document.createElement("p");
    summary.className = "marketplace-card__summary";
    summary.textContent = item.summary || item.description || "";
    body.appendChild(summary);

    const badges = document.createElement("div");
    badges.className = "marketplace-card__badges";
    if (item.active) appendMarketplaceBadge(badges, tr("settings.resource_marketplace.active", "Active"));
    if (item.installed) {
      appendMarketplaceBadge(badges, item.installed_version
        ? formatMessage("settings.resource_marketplace.installed_version", "Installed {version}", { version: item.installed_version })
        : tr("settings.resource_marketplace.installed", "Installed"));
    }
    (Array.isArray(item.tags) ? item.tags : []).forEach((tag) => appendMarketplaceBadge(badges, marketplaceTagLabel(tag)));
    body.appendChild(badges);

    const members = document.createElement("div");
    members.className = "marketplace-card__members";
    appendMarketplaceMemberList(members, tr("settings.resource_marketplace.themes", "Themes"), item.themes || []);
    appendMarketplaceMemberList(members, tr("settings.resource_marketplace.plugins", "Plugins"), item.plugins || []);
    if (members.childNodes.length) body.appendChild(members);

    const actions = document.createElement("div");
    actions.className = "marketplace-card__actions";
    const repo = document.createElement("a");
    repo.className = "marketplace-card__repo";
    repo.href = item.repo || "#";
    repo.target = "_blank";
    repo.rel = "noreferrer";
    repo.textContent = tr("settings.resource_marketplace.view_repo", "Repository");
    actions.appendChild(repo);

    const button = document.createElement("button");
    button.type = "button";
    button.className = "ui-button ui-button--primary";
    button.dataset.resourceMarketplaceInstall = item.id || "";
    button.textContent = marketplaceButtonLabel(item);
    actions.appendChild(button);
    body.appendChild(actions);

    card.appendChild(body);
    resourceMarketplaceList.appendChild(card);
  });
}

function syncResourceMarketplaceFilterButtons() {
  resourceMarketplaceFilterButtons.forEach((button) => {
    button.classList.toggle("is-active", (button.dataset.resourceMarketplaceFilter || "all") === resourceMarketplaceFilter);
  });
}

async function loadResourceMarketplace() {
  if (!resourceMarketplaceList) return;
  setResourceMarketplaceStatus(tr("settings.resource_marketplace.loading", "Loading resource packs"));
  const response = await fetch(apiURL("/admin/api/resource-marketplace"));
  if (!response.ok) {
    setResourceMarketplaceStatus((await response.text()).trim());
    resourceMarketplaceItems = [];
    renderResourceMarketplace();
    return;
  }
  const result = await response.json();
  resourceMarketplaceItems = Array.isArray(result.items) ? result.items : [];
  renderResourceMarketplace();
  setResourceMarketplaceStatus(tr("settings.status.ready", "Ready"));
}

async function installResourceMarketplaceItem(id, button) {
  if (!id) return;
  if (button) button.disabled = true;
  setResourceMarketplaceStatus(tr("settings.resource_marketplace.installing", "Installing"));
  const response = await fetch(apiURL("/admin/api/resource-marketplace/install"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id })
  });
  if (!response.ok) {
    if (button) button.disabled = false;
    setResourceMarketplaceStatus((await response.text()).trim());
    return;
  }
  const result = await response.json();
  const installed = result.installed || {};
  const warnings = Array.isArray(installed.warnings) ? installed.warnings.filter(Boolean) : [];
  const warningTitle = tr("settings.status.installed_with_warnings", "Installed with compatibility warnings");
  if (warnings.length) {
    window.alert([`${warningTitle}:`, ...warnings].join("\n\n"));
  }
  setResourceMarketplaceStatus(warnings.length ? warningTitle : tr("settings.resource_marketplace.installed", "Installed"));
  window.location.reload();
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

/**
 * 按 value 选中一组 radio 中的目标项。
 *
 * @param {string} name - radio 的 name，例如 theme_pack_id 或 theme_locale。
 * @param {string} value - 需要恢复到的已应用值。
 * @returns {boolean} 找到并选中目标项时返回 true；当前页面没有该项时返回 false。
 */
function checkRadioValue(name, value) {
  const inputs = Array.from(document.querySelectorAll(`input[name="${name}"]`));
  const found = inputs.some((input) => String(input.value || "") === String(value || ""));
  if (!found) return false;
  inputs.forEach((input) => {
    const matched = String(input.value || "") === String(value || "");
    input.checked = matched;
  });
  return true;
}

/**
 * 把主题包页面上的临时选择恢复到当前已应用配置。
 *
 * 设计说明：
 * - 黑色粗边框由 radio 的 checked 状态驱动，表示“准备应用”的表单选择。
 * - 下划线由 is-current 驱动，表示服务端已经生效的主题和语言。
 * - 点击“取消更改”只移动黑框，不提交请求，也不改变下划线代表的已应用状态。
 */
function cancelThemeChangesToCurrentApplied() {
  if (!resourcePackSettingsForm) return;
  const appliedThemePackID = String(committedAppearance.themePackID || "").trim();
  const appliedLocale = String(committedAppearance.themeLocale || "en").trim();
  if (!checkRadioValue("theme_pack_id", appliedThemePackID)) return;

  syncSelectableCards(".pack-card:not(.plugin-card)");
  renderThemeLocales();
  checkRadioValue("theme_locale", appliedLocale);
  syncSelectableCards(".locale-card");
  setPackSettingsStatus("");
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
    return localeCodes(locales);
  } catch (_) {
    return [];
  }
}

function localeCodes(locales) {
  if (!Array.isArray(locales)) return [];
  return locales
    .map((value) => {
      if (value && typeof value === "object") return String(value.Code || value.code || "").trim();
      return String(value || "").trim();
    })
    .filter(Boolean);
}

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
  themeLocales: localeCodes(parseJSONDataset((resourcePackSettingsForm && resourcePackSettingsForm.dataset.currentThemeLocales) || "[]", [])),
  pluginOrder: parseJSONDataset((resourcePackSettingsForm && resourcePackSettingsForm.dataset.currentPluginOrder) || "[]", [])
};

function currentThemeLocales(input) {
  if (!input) return [];
  if (input.value === committedAppearance.themePackID && committedAppearance.themeLocales.length) {
    return committedAppearance.themeLocales.slice();
  }
  return parseThemeLocales(input);
}

function renderThemeLocales() {
  if (!themeLocaleGrid) return;
  const input = selectedThemeInput();
  const locales = currentThemeLocales(input);
  const defaultLocale = String((input && input.dataset.defaultLocale) || "en");
  const appliedLocale = String(committedAppearance.themeLocale || "").trim();
  let currentLocale = selectedValue("theme_locale");

  if (!locales.includes(currentLocale)) {
    currentLocale = locales.includes(defaultLocale) ? defaultLocale : (locales[0] || defaultLocale);
  }

  themeLocaleGrid.innerHTML = "";
  locales.forEach((code) => {
    const card = document.createElement("label");
    // is-selected 表示当前表单选择，is-current 表示服务端已经生效的语言。
    card.className = `locale-card${code === currentLocale ? " is-selected" : ""}${code === appliedLocale ? " is-current" : ""}`;
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
    description: node.dataset.pluginDescription || "",
    settingsURL: node.dataset.pluginSettingsUrl || ""
  };
});

let pluginOrder = committedAppearance.pluginOrder.slice();

let selectedQueuePluginID = pluginOrder[0] || "";
let selectedCatalogPluginID = selectedQueuePluginID ? "" : (pluginCards[0] ? pluginCards[0].dataset.pluginId : "");

function renderPluginCards() {
  if (!pluginPackGrid) return;

  pluginPackGrid.innerHTML = "";
  const availablePluginIDs = pluginCards
    .map((card) => card.dataset.pluginId)
    .filter((pluginID) => !pluginOrder.includes(pluginID));

  if (selectedCatalogPluginID && !availablePluginIDs.includes(selectedCatalogPluginID)) {
    selectedCatalogPluginID = "";
  }
  if (!selectedCatalogPluginID && !selectedQueuePluginID) {
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
    selectedQueuePluginID = "";
  }
  if (!pluginOrder.length) {
    return;
  }
  if (!selectedQueuePluginID && !selectedCatalogPluginID) {
    selectedQueuePluginID = pluginOrder[0] || "";
  }

  pluginOrder.forEach((pluginID, index) => {
    const plugin = pluginRegistry[pluginID];
    if (!plugin) return;

    const item = document.createElement("article");
    item.className = `pack-card plugin-card plugin-catalog-card plugin-queue-item${pluginID === selectedQueuePluginID ? " is-selected" : ""}${plugin.settingsURL ? " has-settings" : ""}`;
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

    if (plugin.settingsURL) {
      const settingsLink = document.createElement("a");
      settingsLink.className = "plugin-settings-link";
      settingsLink.href = plugin.settingsURL;
      settingsLink.title = "Plugin settings";
      settingsLink.setAttribute("aria-label", `Plugin settings for ${plugin.name}`);
      const icon = document.createElement("span");
      icon.className = "plugin-settings-link__icon";
      icon.setAttribute("aria-hidden", "true");
      settingsLink.appendChild(icon);
      copy.appendChild(settingsLink);
    }

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
  selectedCatalogPluginID = "";
  selectedQueuePluginID = pluginID;
  renderPluginCards();
}

function removePlugin(pluginID) {
  const index = pluginOrder.indexOf(pluginID);
  pluginOrder = pluginOrder.filter((id) => id !== pluginID);
  if (selectedQueuePluginID === pluginID) {
    selectedQueuePluginID = pluginOrder[index] || pluginOrder[index - 1] || "";
  }
  if (!selectedQueuePluginID && !selectedCatalogPluginID) {
    const nextAvailable = pluginCards.find((card) => !pluginOrder.includes(card.dataset.pluginId));
    selectedCatalogPluginID = nextAvailable ? nextAvailable.dataset.pluginId : "";
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
  selectedCatalogPluginID = "";
  selectedQueuePluginID = pluginID;
  renderPluginCards();
}

/**
 * 把插件包排序列表恢复到当前已应用配置。
 *
 * 参数：无。
 * 返回值：无。
 *
 * 设计说明：
 * 插件页的 `pluginOrder` 是尚未提交的前端队列。用户可能把插件加入队列、移出队列或调整顺序，
 * 但在点击“应用插件包”前这些都只是临时更改；点击“取消更改”时恢复服务端下发的
 * committedAppearance.pluginOrder，并重新渲染可用列表与排序列表。
 */
function cancelPluginChangesToCurrentApplied() {
  pluginOrder = committedAppearance.pluginOrder.slice();
  selectedQueuePluginID = pluginOrder[0] || "";
  selectedCatalogPluginID = selectedQueuePluginID ? "" : (pluginCards[0] ? pluginCards[0].dataset.pluginId : "");
  renderPluginCards();
  setPackSettingsStatus("");
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
    selectedQueuePluginID = "";
    renderPluginCards();
  });
  renderPluginCards();
}

if (pluginQueueList) {
  pluginQueueList.addEventListener("click", (event) => {
    if (event.target.closest(".plugin-settings-link")) return;
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
    selectedCatalogPluginID = "";
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

if (resourceMarketplaceList) {
  resourceMarketplaceList.addEventListener("click", (event) => {
    const button = event.target.closest("[data-resource-marketplace-install]");
    if (!button) return;
    installResourceMarketplaceItem(button.dataset.resourceMarketplaceInstall || "", button);
  });
  loadResourceMarketplace();
}

if (resourceMarketplaceFilterButtons.length) {
  resourceMarketplaceFilterButtons.forEach((button) => {
    button.addEventListener("click", () => {
      resourceMarketplaceFilter = button.dataset.resourceMarketplaceFilter || "all";
      syncResourceMarketplaceFilterButtons();
      renderResourceMarketplace();
    });
  });
  syncResourceMarketplaceFilterButtons();
}

if (resourceMarketplaceRefresh) {
  resourceMarketplaceRefresh.addEventListener("click", () => loadResourceMarketplace());
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

if (cancelChangesButtons.length) {
  cancelChangesButtons.forEach((button) => {
    button.addEventListener("click", () => {
      const scope = button.dataset.cancelScope || "theme";
      if (scope === "theme") {
        cancelThemeChangesToCurrentApplied();
        return;
      }
      if (scope === "plugin") {
        cancelPluginChangesToCurrentApplied();
      }
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
    const result = await response.json();
    const warnings = Array.isArray(result.warnings) ? result.warnings.filter(Boolean) : [];
    const warningTitle = tr("settings.status.installed_with_warnings", "Installed with compatibility warnings");
    if (warnings.length) {
      window.alert([`${warningTitle}:`, ...warnings].join("\n\n"));
    }
    setPackSettingsStatus(warnings.length ? warningTitle : tr("settings.status.installed", "Installed"));
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

if (timeZoneSettingsForm && siteTimeZone) {
  timeZoneSettingsForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const timeZone = String(siteTimeZone.value || "").trim();
    setTimeZoneStatus(tr("settings.status.saving", "Saving"));
    const response = await fetch(apiURL("/admin/api/settings/time-zone"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ time_zone: timeZone })
    });
    if (!response.ok) {
      setTimeZoneStatus((await response.text()).trim());
      return;
    }
    const result = await response.json();
    if (result.time_zone) siteTimeZone.value = result.time_zone;
    setTimeZoneStatus(tr("settings.status.saved", "Saved"));
  });
}

if (siteTitleSettingsForm && siteTitleMain && siteTitleSubtitle) {
  siteTitleSettingsForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    setSiteTitleStatus(tr("settings.status.saving", "Saving"));
    const response = await fetch(apiURL("/admin/api/settings/site-title"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        main: String(siteTitleMain.value || "").trim(),
        subtitle: String(siteTitleSubtitle.value || "").trim()
      })
    });
    if (!response.ok) {
      setSiteTitleStatus((await response.text()).trim());
      return;
    }
    const result = await response.json();
    if (typeof result.main === "string") siteTitleMain.value = result.main;
    if (typeof result.subtitle === "string") siteTitleSubtitle.value = result.subtitle;
    setSiteTitleStatus(tr("settings.status.saved", "Saved"));
  });
}

if (adminAccountSettingsForm && adminAccountUsername && adminAccountCurrentPassword && adminAccountNewPassword && adminAccountConfirmPassword) {
  adminAccountSettingsForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const username = String(adminAccountUsername.value || "").trim();
    const currentPassword = String(adminAccountCurrentPassword.value || "");
    const newPassword = String(adminAccountNewPassword.value || "");
    const confirmPassword = String(adminAccountConfirmPassword.value || "");
    if (!username) {
      setAdminAccountStatus(tr("settings.admin_account.error_username", "Username is required"));
      adminAccountUsername.focus();
      return;
    }
    if (!currentPassword) {
      setAdminAccountStatus(tr("settings.admin_account.error_current_password", "Current password is required"));
      adminAccountCurrentPassword.focus();
      return;
    }
    if ((newPassword || confirmPassword) && newPassword !== confirmPassword) {
      setAdminAccountStatus(tr("settings.admin_account.error_password_match", "New passwords do not match"));
      adminAccountConfirmPassword.focus();
      return;
    }
    setAdminAccountStatus(tr("settings.status.saving", "Saving"));
    const response = await fetch(apiURL("/admin/api/settings/admin-account"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        username,
        current_password: currentPassword,
        new_password: newPassword,
        confirm_password: confirmPassword
      })
    });
    if (!response.ok) {
      setAdminAccountStatus((await response.text()).trim());
      return;
    }
    const result = await response.json();
    if (typeof result.username === "string") adminAccountUsername.value = result.username;
    adminAccountCurrentPassword.value = "";
    adminAccountNewPassword.value = "";
    adminAccountConfirmPassword.value = "";
    setAdminAccountStatus(tr("settings.status.saved", "Saved"));
  });
}

if (permalinkSettingsForm && permalinkPostPattern && permalinkPagePattern && permalinkTagPattern) {
  [permalinkPostPattern, permalinkPagePattern, permalinkTagPattern].forEach((input) => {
    input.addEventListener("input", () => {
      syncPermalinkPreviews();
      if (input === permalinkPostPattern) syncPostPermalinkPreset();
    });
  });
  permalinkPostPresetInputs.forEach((input) => {
    input.addEventListener("change", () => {
      if (!input.checked) return;
      if (input.value === "custom") {
        permalinkPostPattern.focus();
        return;
      }
      permalinkPostPattern.value = input.value;
      syncPermalinkPreviews();
      syncPostPermalinkPreset();
    });
  });
  permalinkTokenButtons.forEach((button) => {
    button.addEventListener("click", () => {
      const active = document.activeElement;
      const target = [permalinkPostPattern, permalinkPagePattern, permalinkTagPattern].includes(active) ? active : permalinkPostPattern;
      insertPermalinkToken(target, button.dataset.permalinkToken || "");
    });
  });
  syncPermalinkPreviews();
  syncPostPermalinkPreset();

  permalinkSettingsForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    setPermalinkStatus(tr("settings.status.saving", "Saving"));
    const response = await fetch(apiURL("/admin/api/settings/permalinks"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        post: String(permalinkPostPattern.value || "").trim(),
        page: String(permalinkPagePattern.value || "").trim(),
        tag: String(permalinkTagPattern.value || "").trim()
      })
    });
    if (!response.ok) {
      setPermalinkStatus((await response.text()).trim());
      return;
    }
    const result = await response.json();
    if (typeof result.post === "string") permalinkPostPattern.value = result.post;
    if (typeof result.page === "string") permalinkPagePattern.value = result.page;
    if (typeof result.tag === "string") permalinkTagPattern.value = result.tag;
    syncPermalinkPreviews();
    setPermalinkStatus(tr("settings.status.saved", "Saved"));
  });
}

if (autoUpdateSettingsForm && autoUpdateEnabled) {
  autoUpdateSettingsForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    setAutoUpdateStatus(tr("settings.status.saving", "Saving"));
    const response = await fetch(apiURL("/admin/api/settings/auto-update"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        enabled: Boolean(autoUpdateEnabled.checked)
      })
    });
    if (!response.ok) {
      setAutoUpdateStatus((await response.text()).trim());
      return;
    }
    const result = await response.json();
    autoUpdateEnabled.checked = Boolean(result.enabled);
    setAutoUpdateStatus(tr("settings.status.saved", "Saved"));
  });
}

if (commentSettingsForm && commentsEnabled) {
  commentSettingsForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    setCommentSettingsStatus(tr("settings.status.saving", "Saving"));
    const response = await fetch(apiURL("/admin/api/settings/comments"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        enabled: Boolean(commentsEnabled.checked)
      })
    });
    if (!response.ok) {
      setCommentSettingsStatus((await response.text()).trim());
      return;
    }
    const result = await response.json();
    commentsEnabled.checked = Boolean(result.enabled);
    setCommentSettingsStatus(tr("settings.status.saved", "Saved"));
  });
}

if (homePageSettingsForm && homePagePageSize) {
  homePageSettingsForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const pageSize = normalizedHomePageSize(homePagePageSize.value);
    homePagePageSize.value = String(pageSize);
    setHomePageSettingsStatus(tr("settings.status.saving", "Saving"));
    const response = await fetch(apiURL("/admin/api/settings/home-page"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        page_size: pageSize
      })
    });
    if (!response.ok) {
      setHomePageSettingsStatus((await response.text()).trim());
      return;
    }
    const result = await response.json();
    homePagePageSize.value = String(normalizedHomePageSize(result.page_size));
    setHomePageSettingsStatus(tr("settings.status.saved", "Saved"));
  });
}

function syncMediaQuality(source) {
  const quality = normalizedQuality(source && source.value);
  if (mediaWebPQuality) mediaWebPQuality.value = String(quality);
  if (mediaWebPQualityRange) mediaWebPQualityRange.value = String(quality);
}

if (mediaWebPQuality && mediaWebPQualityRange) {
  mediaWebPQuality.addEventListener("input", () => syncMediaQuality(mediaWebPQuality));
  mediaWebPQualityRange.addEventListener("input", () => syncMediaQuality(mediaWebPQualityRange));
  mediaWebPQuality.addEventListener("change", () => syncMediaQuality(mediaWebPQuality));
  mediaWebPQualityRange.addEventListener("change", () => syncMediaQuality(mediaWebPQualityRange));
}

if (mediaProcessingSettingsForm) {
  mediaProcessingSettingsForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    syncMediaQuality(mediaWebPQuality || mediaWebPQualityRange);
    setMediaProcessingStatus(tr("settings.status.saving", "Saving"));
    const response = await fetch(apiURL("/admin/api/settings/media-processing"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        auto_webp: Boolean(mediaAutoWebP && mediaAutoWebP.checked),
        webp_quality: normalizedQuality(mediaWebPQuality && mediaWebPQuality.value),
        keep_original: Boolean(mediaKeepOriginal && mediaKeepOriginal.checked)
      })
    });
    if (!response.ok) {
      setMediaProcessingStatus((await response.text()).trim());
      return;
    }
    setMediaProcessingStatus(tr("settings.status.saved", "Saved"));
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
