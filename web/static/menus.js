const themeMenuRoot = document.querySelector("#themeMenuSettings");
const themeMenuStatus = document.querySelector("#themeMenuStatus");
const menuSelector = document.querySelector("#menuSelector");
const selectThemeMenuButton = document.querySelector("#selectThemeMenu");
const addThemeMenuButton = document.querySelector("#addThemeMenu");
const deleteThemeMenuButton = document.querySelector("#deleteThemeMenu");
const saveThemeMenusButton = document.querySelector("#saveThemeMenus");
const menuNameInput = document.querySelector("#menuNameInput");
const menuSourcePanels = document.querySelector("#menuSourcePanels");
const themeMenuList = document.querySelector("#themeMenuList");

function tr(key, fallback) {
  if (window.postizerMessage) return window.postizerMessage(key, fallback);
  return fallback;
}

function apiURL(path) {
  const token = new URLSearchParams(window.location.search).get("token");
  if (!token) return path;
  const url = new URL(path, window.location.origin);
  url.searchParams.set("token", token);
  return url.pathname + url.search;
}

function setThemeMenuStatus(message) {
  if (themeMenuStatus) themeMenuStatus.textContent = message || "";
}

function readInitialData() {
  const script = document.querySelector("#themeMenuInitialData");
  if (!script) return {};
  try {
    return JSON.parse(script.textContent || "{}");
  } catch (_) {
    return {};
  }
}

function slugID(value, fallback = "menu") {
  const id = String(value || "")
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .replace(/-+/g, "-");
  return id || fallback;
}

function uniqueMenuID(base) {
  const used = new Set(state.menus.map((menu) => menu.id));
  let candidate = slugID(base, "menu");
  const root = candidate;
  for (let suffix = 2; used.has(candidate); suffix += 1) {
    candidate = `${root}-${suffix}`;
  }
  return candidate;
}

function menuTypeLabel(type) {
  switch (type) {
    case "page": return tr("menus.item.type.page", "Page");
    case "post": return tr("menus.item.type.post", "Post");
    case "tag": return tr("menus.item.type.tag", "Tag");
    case "custom": return tr("menus.item.type.custom", "Custom Link");
    default: return type;
  }
}

function optionsForType(type) {
  switch (type) {
    case "page": return data.options.pages || [];
    case "post": return data.options.posts || [];
    case "tag": return data.options.tags || [];
    default: return [];
  }
}

function optionTitle(type, slug) {
  const option = optionsForType(type).find((entry) => entry.slug === slug);
  return option ? option.title : slug;
}

function normalizeState(settings = {}) {
  const menus = Array.isArray(settings.menus) ? settings.menus : [];
  const normalizedMenus = menus.map((menu, index) => ({
    id: slugID(menu.id || menu.name, `menu-${index + 1}`),
    name: String(menu.name || menu.id || `Menu ${index + 1}`).trim(),
    items: Array.isArray(menu.items) ? menu.items.map((item) => ({
      type: ["page", "post", "tag", "custom"].includes(item.type) ? item.type : "custom",
      label: String(item.label || ""),
      target: String(item.target || ""),
      url: String(item.url || "")
    })) : []
  }));
  return { menus: normalizedMenus, selectedIndex: normalizedMenus.length ? 0 : -1 };
}

const data = readInitialData();
const state = normalizeState(data.settings || {});
data.options = data.options || {};

function currentMenu() {
  if (state.selectedIndex < 0 || state.selectedIndex >= state.menus.length) return null;
  return state.menus[state.selectedIndex];
}

function renderMenuSelector() {
  if (!menuSelector) return;
  menuSelector.innerHTML = "";
  if (!state.menus.length) {
    menuSelector.appendChild(new Option(tr("menus.menu.empty", "No custom menus yet."), ""));
    menuSelector.disabled = true;
    return;
  }
  menuSelector.disabled = false;
  state.menus.forEach((menu, index) => {
    menuSelector.appendChild(new Option(menu.name || menu.id, String(index)));
  });
  menuSelector.value = String(state.selectedIndex);
}

function renderSourcePanels() {
  if (!menuSourcePanels) return;
  menuSourcePanels.innerHTML = "";
  [
    { type: "page", title: tr("menus.source.pages", "Pages") },
    { type: "post", title: tr("menus.source.posts", "Posts") },
    { type: "tag", title: tr("menus.source.tags", "Tags") },
    { type: "custom", title: tr("menus.source.custom", "Custom Link") }
  ].forEach((source) => {
    menuSourcePanels.appendChild(source.type === "custom" ? renderCustomSource(source) : renderOptionSource(source));
  });
}

function renderOptionSource(source) {
  const panel = document.createElement("section");
  panel.className = "menu-source-panel is-collapsed";
  const summary = document.createElement("button");
  summary.type = "button";
  summary.className = "menu-source-panel__head";
  const title = document.createElement("span");
  title.textContent = source.title;
  const indicator = document.createElement("span");
  indicator.className = "menu-source-panel__indicator";
  indicator.setAttribute("aria-hidden", "true");
  summary.append(title, indicator);
  const body = document.createElement("div");
  body.className = "menu-source-panel__body";
  const list = document.createElement("div");
  list.className = "menu-source-list";
  const options = optionsForType(source.type);
  if (!options.length) {
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = tr("menus.source.empty", "No items available.");
    list.appendChild(empty);
  }
  options.forEach((option) => {
    const row = document.createElement("label");
    row.className = "menu-source-option";
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.value = option.slug;
    const text = document.createElement("span");
    text.textContent = option.title;
    row.append(checkbox, text);
    list.appendChild(row);
  });
  const add = document.createElement("button");
  add.type = "button";
  add.className = "ui-button";
  add.textContent = tr("menus.source.add_selected", "Add to Menu");
  add.addEventListener("click", () => {
    const menu = currentMenu();
    if (!menu) return;
    list.querySelectorAll("input:checked").forEach((checkbox) => {
      menu.items.push({ type: source.type, label: "", target: checkbox.value, url: "" });
      checkbox.checked = false;
    });
    renderMenuStructure();
  });
  body.append(list, add);
  summary.addEventListener("click", () => panel.classList.toggle("is-collapsed"));
  panel.append(summary, body);
  return panel;
}

function renderCustomSource(source) {
  const panel = document.createElement("section");
  panel.className = "menu-source-panel is-collapsed";
  const summary = document.createElement("button");
  summary.type = "button";
  summary.className = "menu-source-panel__head";
  const title = document.createElement("span");
  title.textContent = source.title;
  const indicator = document.createElement("span");
  indicator.className = "menu-source-panel__indicator";
  indicator.setAttribute("aria-hidden", "true");
  summary.append(title, indicator);
  const body = document.createElement("div");
  body.className = "menu-source-panel__body";
  const url = document.createElement("input");
  url.className = "ui-input";
  url.placeholder = tr("menus.source.custom_url", "URL");
  url.value = "/";
  const label = document.createElement("input");
  label.className = "ui-input";
  label.placeholder = tr("menus.source.link_text", "Link text");
  const add = document.createElement("button");
  add.type = "button";
  add.className = "ui-button";
  add.textContent = tr("menus.source.add_selected", "Add to Menu");
  add.addEventListener("click", () => {
    const menu = currentMenu();
    if (!menu) return;
    const nextURL = String(url.value || "").trim();
    if (!nextURL) return;
    menu.items.push({ type: "custom", label: String(label.value || "").trim(), target: "", url: nextURL });
    label.value = "";
    renderMenuStructure();
  });
  body.append(url, label, add);
  summary.addEventListener("click", () => panel.classList.toggle("is-collapsed"));
  panel.append(summary, body);
  return panel;
}

function renderMenuStructure() {
  const menu = currentMenu();
  if (menuNameInput) {
    menuNameInput.disabled = !menu;
    menuNameInput.value = menu ? menu.name : "";
  }
  if (deleteThemeMenuButton) deleteThemeMenuButton.disabled = !menu;
  if (!themeMenuList) return;
  themeMenuList.innerHTML = "";
  if (!menu) {
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = tr("menus.menu.empty", "No custom menus yet.");
    themeMenuList.appendChild(empty);
    return;
  }
  if (!menu.items.length) {
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = tr("menus.structure.empty", "This menu has no items.");
    themeMenuList.appendChild(empty);
    return;
  }
  menu.items.forEach((item, itemIndex) => {
    themeMenuList.appendChild(renderMenuItem(menu, item, itemIndex));
  });
}

function renderMenuItem(menu, item, itemIndex) {
  const row = document.createElement("article");
  row.className = "menu-structure-item";
  const head = document.createElement("div");
  head.className = "menu-structure-item__head";
  const title = document.createElement("strong");
  title.textContent = item.label || item.url || optionTitle(item.type, item.target);
  const typeLabel = document.createElement("span");
  typeLabel.textContent = menuTypeLabel(item.type);
  const tools = document.createElement("span");
  tools.className = "menu-structure-item__tools";
  const up = itemButton(tr("menus.item.move_up", "Up"), () => {
    if (itemIndex <= 0) return;
    const previous = menu.items[itemIndex - 1];
    menu.items[itemIndex - 1] = item;
    menu.items[itemIndex] = previous;
    renderMenuStructure();
  });
  const down = itemButton(tr("menus.item.move_down", "Down"), () => {
    if (itemIndex >= menu.items.length - 1) return;
    const next = menu.items[itemIndex + 1];
    menu.items[itemIndex + 1] = item;
    menu.items[itemIndex] = next;
    renderMenuStructure();
  });
  const remove = itemButton(tr("common.delete", "Delete"), () => {
    menu.items.splice(itemIndex, 1);
    renderMenuStructure();
  });
  tools.append(up, down, remove);
  head.append(title, typeLabel, tools);

  const fields = document.createElement("div");
  fields.className = "menu-structure-item__fields";
  const label = document.createElement("input");
  label.className = "ui-input";
  label.value = item.label;
  label.placeholder = tr("menus.item.label", "Label (optional)");
  label.addEventListener("input", () => {
    item.label = label.value;
    title.textContent = item.label || item.url || optionTitle(item.type, item.target);
  });
  const target = item.type === "custom" ? document.createElement("input") : document.createElement("select");
  target.className = "ui-input";
  if (item.type === "custom") {
    target.value = item.url;
    target.placeholder = tr("menus.item.url", "https://example.com or /path");
    target.addEventListener("input", () => {
      item.url = target.value;
      title.textContent = item.label || item.url || optionTitle(item.type, item.target);
    });
  } else {
    optionsForType(item.type).forEach((option) => target.appendChild(new Option(option.title, option.slug)));
    target.value = item.target;
    target.addEventListener("change", () => {
      item.target = target.value;
      title.textContent = item.label || item.url || optionTitle(item.type, item.target);
    });
  }
  fields.append(label, target);
  row.append(head, fields);
  return row;
}

function itemButton(label, onClick) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "admin-inline-delete__button";
  button.textContent = label;
  button.addEventListener("click", onClick);
  return button;
}

function selectMenu(index) {
  if (state.menus.length === 0) state.selectedIndex = -1;
  else state.selectedIndex = Math.min(Math.max(index, 0), state.menus.length - 1);
  renderAll();
}

function renderAll() {
  renderMenuSelector();
  renderMenuStructure();
  renderSourcePanels();
}

function payload() {
  return {
    menus: state.menus.map((menu) => ({
      id: menu.id,
      name: String(menu.name || "").trim(),
      items: menu.items.map((item) => ({
        type: item.type,
        label: String(item.label || "").trim(),
        target: String(item.target || "").trim(),
        url: String(item.url || "").trim()
      }))
    }))
  };
}

if (selectThemeMenuButton) {
  selectThemeMenuButton.addEventListener("click", () => selectMenu(Number.parseInt(menuSelector.value, 10) || 0));
}

if (menuNameInput) {
  menuNameInput.addEventListener("input", () => {
    const menu = currentMenu();
    if (!menu) return;
    menu.name = menuNameInput.value;
    renderMenuSelector();
  });
}

if (addThemeMenuButton) {
  addThemeMenuButton.addEventListener("click", () => {
    const name = tr("menus.menu.default_name", "New Menu");
    state.menus.push({ id: uniqueMenuID(name), name, items: [] });
    selectMenu(state.menus.length - 1);
  });
}

if (deleteThemeMenuButton) {
  deleteThemeMenuButton.addEventListener("click", async () => {
    if (state.selectedIndex < 0) return;
    const menu = currentMenu();
    if (menu && !window.confirm(tr("menus.confirm.delete", "Delete this menu?"))) return;
    state.menus.splice(state.selectedIndex, 1);
    selectMenu(state.selectedIndex);
    await saveMenus(tr("settings.status.deleting", "Deleting"));
  });
}

async function saveMenus(statusMessage) {
  setThemeMenuStatus(statusMessage || tr("settings.status.saving", "Saving"));
  const response = await fetch(apiURL("/admin/api/menus"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload())
  });
  if (!response.ok) {
    setThemeMenuStatus((await response.text()).trim());
    return false;
  }
  setThemeMenuStatus(tr("settings.status.saved", "Saved"));
  window.location.reload();
  return true;
}

if (saveThemeMenusButton) {
  saveThemeMenusButton.addEventListener("click", async () => {
    await saveMenus();
  });
}

if (themeMenuRoot) renderAll();
