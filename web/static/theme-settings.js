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
  const menuLocations = {};
  Object.entries(settings.menu_locations || {}).forEach(([location, menuID]) => {
    const value = String(menuID || "");
    if (value === "" || menuIDs.has(value)) menuLocations[location] = value;
  });
  return { menus, menu_locations: menuLocations };
}

const data = readInitialData();
const state = normalizeState(data.settings || {});
data.locations = Array.isArray(data.locations) ? data.locations : [];

function renderThemeLocations() {
  if (!themeLocationList) return;
  themeLocationList.innerHTML = "";
  if (!data.locations.length) {
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = tr("theme_settings.locations.empty", "The active theme does not declare menu locations.");
    themeLocationList.appendChild(empty);
    return;
  }
  data.locations.forEach((location) => {
    const label = document.createElement("label");
    label.className = "theme-location-row";
    const copy = document.createElement("span");
    copy.className = "theme-location-row__copy";
    const name = document.createElement("strong");
    name.textContent = locationName(location);
    const description = document.createElement("span");
    description.textContent = locationDescription(location);
    copy.append(name, description);

    const select = document.createElement("select");
    select.className = "ui-input";
    select.dataset.locationId = location.id;
    select.appendChild(new Option(tr("theme_settings.locations.default", "Default menu"), "__default__"));
    select.appendChild(new Option(tr("theme_settings.locations.none", "No menu"), "__none__"));
    state.menus.forEach((menu) => {
      select.appendChild(new Option(menu.name || menu.id, menu.id));
    });
    if (Object.prototype.hasOwnProperty.call(state.menu_locations, location.id)) {
      select.value = state.menu_locations[location.id] === "" ? "__none__" : state.menu_locations[location.id];
    } else {
      select.value = "__default__";
    }
    select.addEventListener("change", () => {
      if (select.value === "__default__") delete state.menu_locations[location.id];
      else if (select.value === "__none__") state.menu_locations[location.id] = "";
      else state.menu_locations[location.id] = select.value;
    });
    label.append(copy, select);
    themeLocationList.appendChild(label);
  });
}

if (saveThemeSettingsButton) {
  saveThemeSettingsButton.addEventListener("click", async () => {
    setThemeSettingsStatus(tr("settings.status.saving", "Saving"));
    const response = await fetch(apiURL("/admin/api/theme-settings"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ menu_locations: { ...state.menu_locations } })
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
