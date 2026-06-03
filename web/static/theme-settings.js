const themeSettingsRoot = document.querySelector("#themeSettings");
const themeSettingsStatus = document.querySelector("#themeSettingsStatus");
const themeLocationList = document.querySelector("#themeLocationList");
const saveThemeSettingsButton = document.querySelector("#saveThemeSettings");

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

function setThemeSettingsStatus(message) {
  if (themeSettingsStatus) themeSettingsStatus.textContent = message || "";
}

function readInitialData() {
  const script = document.querySelector("#themeSettingsInitialData");
  if (!script) return {};
  try {
    return JSON.parse(script.textContent || "{}");
  } catch (_) {
    return {};
  }
}

function locationName(location) {
  return tr(`menus.location.${location.id}`, location.name || location.id);
}

function locationDescription(location) {
  return tr(`menus.location.${location.id}.description`, location.description || location.id);
}

function normalizeState(settings = {}) {
  const menus = Array.isArray(settings.menus) ? settings.menus : [];
  const menuIDs = new Set(menus.map((menu) => String(menu.id || "")));
  menuIDs.add(defaultThemeMenuID);
  const sidebars = Array.isArray(settings.sidebars) ? settings.sidebars : [];
  const sidebarIDs = new Set(sidebars.map((sidebar) => String(sidebar.id || "")));
  const menuLocations = {};
  Object.entries(settings.menu_locations || {}).forEach(([location, menuID]) => {
    const value = String(menuID || "");
    if (value === "" || menuIDs.has(value)) menuLocations[location] = value;
  });
  let sidebar = null;
  if (Object.prototype.hasOwnProperty.call(settings, "sidebar")) {
    const value = String(settings.sidebar || "");
    if (value === "" || sidebarIDs.has(value)) sidebar = value;
  }
  return { menus, menu_locations: menuLocations, sidebars, sidebar };
}

const data = readInitialData();
const defaultThemeMenuID = String(data.default_menu_id || "default-menu");
const state = normalizeState(data.settings || {});
data.locations = Array.isArray(data.locations) ? data.locations : [];

/**
 * 判断主题菜单位置在没有显式保存时是否默认不使用菜单。
 * @param {string} locationID - 菜单位置 ID。
 * @returns {boolean} 页脚默认不使用菜单时返回 true。
 */
function menuLocationDefaultsToNone(locationID) {
  return String(locationID || "") === "footer";
}

function renderThemeLocations() {
  if (!themeLocationList) return;
  themeLocationList.innerHTML = "";
  let sidebarRendered = false;
  data.locations.forEach((location) => {
    if (!sidebarRendered && String(location.id || "") === "footer") {
      themeLocationList.appendChild(renderSidebarLocationRow());
      sidebarRendered = true;
    }
    const select = document.createElement("select");
    select.className = "ui-input";
    select.dataset.locationId = location.id;
    select.appendChild(new Option(tr("theme_settings.locations.default", "Default menu"), "__default__"));
    select.appendChild(new Option(tr("theme_settings.locations.none", "No menu"), "__none__"));
    state.menus.forEach((menu) => {
      select.appendChild(new Option(menu.name || menu.id, menu.id));
    });
    const hasSavedLocation = Object.prototype.hasOwnProperty.call(state.menu_locations, location.id);
    if (hasSavedLocation) {
      const savedValue = state.menu_locations[location.id];
      if (savedValue === "") select.value = "__none__";
      else if (savedValue === defaultThemeMenuID) select.value = "__default__";
      else select.value = savedValue;
    } else {
      select.value = menuLocationDefaultsToNone(location.id) ? "__none__" : "__default__";
    }
    select.addEventListener("change", () => {
      if (select.value === "__default__") {
        if (menuLocationDefaultsToNone(location.id)) state.menu_locations[location.id] = defaultThemeMenuID;
        else delete state.menu_locations[location.id];
      }
      else if (select.value === "__none__") state.menu_locations[location.id] = "";
      else state.menu_locations[location.id] = select.value;
    });
    themeLocationList.appendChild(renderThemeLocationRow(locationName(location), locationDescription(location), select));
  });
  if (!sidebarRendered) themeLocationList.appendChild(renderSidebarLocationRow());
}

/**
 * 渲染主题位置列表中的一行。
 * @param {string} nameText - 左侧位置名称。
 * @param {string} descriptionText - 左侧位置说明。
 * @param {HTMLSelectElement} select - 右侧选择框。
 * @returns {HTMLLabelElement} 返回可挂载到位置列表的行。
 */
function renderThemeLocationRow(nameText, descriptionText, select) {
  const label = document.createElement("label");
  label.className = "theme-location-row";
  const copy = document.createElement("span");
  copy.className = "theme-location-row__copy";
  const name = document.createElement("strong");
  name.textContent = nameText;
  const description = document.createElement("span");
  description.textContent = descriptionText;
  copy.append(name, description);
  label.append(copy, select);
  return label;
}

/**
 * 渲染和导航栏同级的侧边栏投放设置。
 * @returns {HTMLLabelElement} 返回侧边栏选择行。
 */
function renderSidebarLocationRow() {
  const select = document.createElement("select");
  select.className = "ui-input";
  select.dataset.sidebarLocation = "primary";
  select.appendChild(new Option(tr("theme_settings.sidebar.default", "Default sidebar"), "__default__"));
  select.appendChild(new Option(tr("theme_settings.sidebar.none", "No sidebar"), "__none__"));
  state.sidebars.forEach((sidebar) => {
    select.appendChild(new Option(sidebar.name || sidebar.id, sidebar.id));
  });
  if (state.sidebar === "") {
    select.value = "__none__";
  } else if (state.sidebar) {
    select.value = state.sidebar;
  } else {
    select.value = "__default__";
  }
  select.addEventListener("change", () => {
    if (select.value === "__default__") state.sidebar = null;
    else if (select.value === "__none__") state.sidebar = "";
    else state.sidebar = select.value;
  });
  return renderThemeLocationRow(
    tr("theme_settings.sidebar.title", "Sidebar"),
    tr("theme_settings.sidebar.description", "Right rail area"),
    select
  );
}

if (saveThemeSettingsButton) {
  saveThemeSettingsButton.addEventListener("click", async () => {
    setThemeSettingsStatus(tr("settings.status.saving", "Saving"));
    const response = await fetch(apiURL("/admin/api/theme-settings"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ menu_locations: { ...state.menu_locations }, sidebar: state.sidebar })
    });
    if (!response.ok) {
      setThemeSettingsStatus((await response.text()).trim());
      return;
    }
    setThemeSettingsStatus(tr("settings.status.saved", "Saved"));
    window.location.reload();
  });
}

if (themeSettingsRoot) renderThemeLocations();
