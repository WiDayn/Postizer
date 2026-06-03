const themeMenuRoot = document.querySelector("#themeMenuSettings");
const themeMenuStatus = document.querySelector("#themeMenuStatus");
const menuSelector = document.querySelector("#menuSelector");
const menuSelectorToggle = document.querySelector("#menuSelectorToggle");
const menuSelectorList = document.querySelector("#menuSelectorList");
const addThemeMenuButton = document.querySelector("#addThemeMenu");
const deleteThemeMenuButton = document.querySelector("#deleteThemeMenu");
const saveThemeMenusButton = document.querySelector("#saveThemeMenus");
const addThemeMenuItemButton = document.querySelector("#addThemeMenuItem");
const themeMenuItemList = document.querySelector("#themeMenuItemList");
const themeMenuDetailsTitle = document.querySelector("#themeMenuDetailsTitle");
const themeMenuDetailsTitleInput = document.querySelector("#themeMenuDetailsTitleInput");
const themeMenuItemActions = document.querySelector("#themeMenuItemActions");
const themeMenuRenameItemButton = document.querySelector("#themeMenuRenameItem");
const themeMenuDeleteItemButton = document.querySelector("#themeMenuDeleteItem");
const themeMenuDeleteActions = document.querySelector("#themeMenuDeleteActions");
const themeMenuDeleteConfirmButton = document.querySelector("#themeMenuDeleteConfirm");
const themeMenuDeleteCancelButton = document.querySelector("#themeMenuDeleteCancel");
const themeMenuRenameActions = document.querySelector("#themeMenuRenameActions");
const themeMenuRenameSaveButton = document.querySelector("#themeMenuRenameSave");
const themeMenuRenameCancelButton = document.querySelector("#themeMenuRenameCancel");
const themeMenuList = document.querySelector("#themeMenuList");
const saveThemeMenusDefaultLabel = saveThemeMenusButton ? saveThemeMenusButton.textContent : "";
let saveThemeMenusFeedbackTimer = 0;
const themeMenuRenameControls = {
  title: themeMenuDetailsTitle,
  input: themeMenuDetailsTitleInput,
  container: themeMenuItemActions,
  rename: themeMenuRenameItemButton,
  remove: themeMenuDeleteItemButton,
  deleteActions: themeMenuDeleteActions,
  deleteConfirm: themeMenuDeleteConfirmButton,
  deleteCancel: themeMenuDeleteCancelButton,
  actions: themeMenuRenameActions,
  save: themeMenuRenameSaveButton,
  cancel: themeMenuRenameCancelButton
};

const customSidebarRoot = document.querySelector("#customSidebarSettings");
const customSidebarStatus = document.querySelector("#customSidebarStatus");
const sidebarSectionSelector = document.querySelector("#sidebarSectionSelector");
const sidebarSectionSelectorToggle = document.querySelector("#sidebarSectionSelectorToggle");
const sidebarSectionSelectorList = document.querySelector("#sidebarSectionSelectorList");
const addSidebarSectionButton = document.querySelector("#addSidebarSection");
const deleteSidebarSectionButton = document.querySelector("#deleteSidebarSection");
const saveCustomSidebarButton = document.querySelector("#saveCustomSidebar");
const addSidebarSectionItemButton = document.querySelector("#addSidebarSectionItem");
const sidebarSectionItemList = document.querySelector("#sidebarSectionItemList");
const sidebarSectionDetailsTitle = document.querySelector("#sidebarSectionDetailsTitle");
const sidebarSectionDetailsTitleInput = document.querySelector("#sidebarSectionDetailsTitleInput");
const sidebarSectionItemActions = document.querySelector("#sidebarSectionItemActions");
const sidebarSectionRenameItemButton = document.querySelector("#sidebarSectionRenameItem");
const sidebarSectionDeleteItemButton = document.querySelector("#sidebarSectionDeleteItem");
const sidebarSectionDeleteActions = document.querySelector("#sidebarSectionDeleteActions");
const sidebarSectionDeleteConfirmButton = document.querySelector("#sidebarSectionDeleteConfirm");
const sidebarSectionDeleteCancelButton = document.querySelector("#sidebarSectionDeleteCancel");
const sidebarSectionRenameActions = document.querySelector("#sidebarSectionRenameActions");
const sidebarSectionRenameSaveButton = document.querySelector("#sidebarSectionRenameSave");
const sidebarSectionRenameCancelButton = document.querySelector("#sidebarSectionRenameCancel");
const sidebarSectionList = document.querySelector("#sidebarSectionList");
const saveCustomSidebarDefaultLabel = saveCustomSidebarButton ? saveCustomSidebarButton.textContent : "";
let saveCustomSidebarFeedbackTimer = 0;
const sidebarSectionRenameControls = {
  title: sidebarSectionDetailsTitle,
  input: sidebarSectionDetailsTitleInput,
  container: sidebarSectionItemActions,
  rename: sidebarSectionRenameItemButton,
  remove: sidebarSectionDeleteItemButton,
  deleteActions: sidebarSectionDeleteActions,
  deleteConfirm: sidebarSectionDeleteConfirmButton,
  deleteCancel: sidebarSectionDeleteCancelButton,
  actions: sidebarSectionRenameActions,
  save: sidebarSectionRenameSaveButton,
  cancel: sidebarSectionRenameCancelButton
};

function tr(key, fallback) {
  if (window.postizerMessage) return window.postizerMessage(key, fallback);
  return fallback;
}

/**
 * 读取带占位符的翻译文案。
 * @param {string} key - 翻译 key。
 * @param {string} fallback - 翻译缺失时使用的英文文案。
 * @param {Record<string, string|number>} values - 需要替换的占位符值。
 * @returns {string} 返回替换 `{name}` 这类占位符后的文案。
 */
function trFormat(key, fallback, values = {}) {
  let text = tr(key, fallback);
  Object.entries(values).forEach(([name, value]) => {
    text = text.split(`{${name}}`).join(String(value));
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

function setThemeMenuStatus(message) {
  if (themeMenuStatus) themeMenuStatus.textContent = message || "";
}

function setCustomSidebarStatus(message) {
  if (customSidebarStatus) customSidebarStatus.textContent = message || "";
}

function setSaveThemeMenusFeedback(message, options = {}) {
  if (!saveThemeMenusButton) return;
  window.clearTimeout(saveThemeMenusFeedbackTimer);
  saveThemeMenusButton.textContent = message || saveThemeMenusDefaultLabel;
  setButtonDisabled(saveThemeMenusButton, Boolean(options.disabled) || !canSaveCurrentThemeMenu());
  if (options.resetAfter) {
    saveThemeMenusFeedbackTimer = window.setTimeout(() => {
      saveThemeMenusButton.textContent = saveThemeMenusDefaultLabel;
      setButtonDisabled(saveThemeMenusButton, !canSaveCurrentThemeMenu());
    }, options.resetAfter);
  }
}

function setSaveCustomSidebarFeedback(message, options = {}) {
  if (!saveCustomSidebarButton) return;
  window.clearTimeout(saveCustomSidebarFeedbackTimer);
  saveCustomSidebarButton.textContent = message || saveCustomSidebarDefaultLabel;
  setButtonDisabled(saveCustomSidebarButton, Boolean(options.disabled) || !canSaveCurrentCustomSidebar());
  if (options.resetAfter) {
    saveCustomSidebarFeedbackTimer = window.setTimeout(() => {
      saveCustomSidebarButton.textContent = saveCustomSidebarDefaultLabel;
      setButtonDisabled(saveCustomSidebarButton, !canSaveCurrentCustomSidebar());
    }, options.resetAfter);
  }
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

function uniqueSidebarID(base) {
  const used = new Set(state.sidebars.map((sidebar) => sidebar.id));
  let candidate = slugID(base, "sidebar");
  const root = candidate;
  for (let suffix = 2; used.has(candidate); suffix += 1) {
    candidate = `${root}-${suffix}`;
  }
  return candidate;
}

function optionsForType(type) {
  switch (type) {
    case "page": return data.options.pages || [];
    case "post": return data.options.posts || [];
    case "tag": return data.options.tags || [];
    default: return [];
  }
}

const systemFeatureOptions = [
  { value: "home", label: tr("menus.feature.home", "Front Page"), url: "/" },
  { value: "archive", label: tr("menus.feature.archive", "Archive"), url: "/archive" },
  { value: "tags", label: tr("menus.feature.tags", "Tags Index"), url: "/tags" },
  { value: "search", label: tr("menus.feature.search", "Search"), url: "/search" },
  { value: "admin", label: tr("menus.feature.admin", "Admin"), url: "/admin" }
];

const sidebarBlockTypeOptions = [
  { value: "topics", label: tr("site.right_rail.topics", "Topics") },
  { value: "pages", label: tr("site.right_rail.pages", "Pages") },
  { value: "feeds", label: tr("site.right_rail.feeds", "Feeds") },
  { value: "recent-posts", label: tr("site.right_rail.recent_posts", "Recent Posts") }
];

function systemFeatureTypes() {
  return new Set(systemFeatureOptions.map((feature) => feature.value));
}

function sidebarBlockTypes() {
  return new Set(["", ...sidebarBlockTypeOptions.map((option) => option.value), "custom"]);
}

function featureByURL(url) {
  return systemFeatureOptions.find((feature) => feature.url === String(url || "").trim()) || null;
}

function featureByType(type) {
  const normalizedType = String(type || "").trim();
  return systemFeatureOptions.find((feature) => feature.value === normalizedType) || null;
}

function featureByValue(value) {
  return systemFeatureOptions.find((feature) => feature.value === value) || systemFeatureOptions[0];
}

/**
 * 清洗侧边栏区块类型。
 * @param {string} value - 后端返回或用户选择的区块类型。
 * @returns {string} 返回受支持的区块类型；未知类型按自定义区域处理。
 */
function normalizeSidebarBlockType(value) {
  const type = String(value || "").trim().toLowerCase();
  return sidebarBlockTypes().has(type) ? type : "custom";
}

/**
 * 读取侧边栏区块类型的显示名。
 * @param {string} type - 侧边栏区块类型。
 * @returns {string} 返回后台列表和详情使用的类型名称。
 */
function sidebarBlockTypeLabel(type) {
  const normalizedType = normalizeSidebarBlockType(type);
  if (normalizedType === "") return tr("sidebar.type.none", "No function");
  if (normalizedType === "custom") return tr("sidebar.type.custom", "Custom Area");
  const option = sidebarBlockTypeOptions.find((entry) => entry.value === normalizedType);
  return option ? option.label : tr("site.right_rail.topics", "Topics");
}

/**
 * 读取侧边栏区块编辑模式。
 * @param {{type: string}|null} block - 当前侧边栏区块。
 * @returns {string} 返回空字符串、auto 或 custom。
 */
function sidebarBlockMode(block) {
  const type = normalizeSidebarBlockType(block && block.type);
  if (type === "") return "";
  return type === "custom" ? "custom" : "auto";
}

function generatedFeatureLabels() {
  return new Set(systemFeatureOptions.map((feature) => feature.label));
}

function optionTitle(type, slug) {
  const option = optionsForType(type).find((entry) => entry.slug === slug);
  return option ? option.title : slug;
}

function itemTitle(item) {
  if (item && sidebarBlockTypes().has(String(item.type || ""))) {
    return item.label || sidebarBlockTypeLabel(item.type);
  }
  const feature = featureByType(item.type) || (item.type === "url" ? featureByURL(item.url) : null);
  if (feature) return item.label || feature.label;
  return item.label || item.url || optionTitle(item.type, item.target);
}

function currentItemMode(item) {
  if (!item || !item.type) return "";
  if (systemFeatureTypes().has(item.type)) return "feature";
  if (["page", "post", "tag"].includes(item.type)) return "content";
  if (item.type === "url") {
    const feature = featureByURL(item.url);
    return feature ? "feature" : "link";
  }
  return "";
}

function firstAvailableContentType() {
  return ["page", "post", "tag"].find((type) => optionsForType(type).length > 0) || "page";
}

function firstOptionSlug(type) {
  const options = optionsForType(type);
  return options.length ? options[0].slug : "";
}

function createBlankMenuItem(items) {
  const nextNumber = nextNewMenuItemNumber(items);
  return {
    type: "",
    label: newItemLabel(nextNumber),
    target: "",
    url: ""
  };
}

/**
 * 创建一个新的侧边栏区块。
 * @param {Array<{label: string}>} items - 当前侧边栏已有区块。
 * @returns {{type: string, label: string, items: Array}} 返回默认无功能区块。
 */
function createBlankSidebarBlock(items) {
  const nextNumber = nextNewMenuItemNumber(items);
  return {
    type: "",
    label: newItemLabel(nextNumber),
    items: []
  };
}

/**
 * 在改变菜单项目标前保留当前显示名。
 *
 * 设计说明：
 * 旧数据可能没有显式 label，左侧名称会从 page/post/tag 目标标题推导出来。
 * 如果直接切换目标，左侧看起来就像“名字被改了”。这里在目标变化前把
 * 当前显示名写入 label，后续目标怎么变都不会影响用户看到的名字。
 *
 * @param {{type: string, label: string, target: string, url: string}} item - 当前菜单项或侧边栏项。
 * @returns {void}
 */
function preserveVisibleLabel(item) {
  if (!item || String(item.label || "").trim()) return;
  const title = String(itemTitle(item) || "").trim();
  if (title) item.label = title;
}

/**
 * 更新结构详情面板标题。
 * @param {HTMLElement|null} element - 要写入标题的 h2 节点。
 * @param {string} value - 当前选中菜单项或侧边栏项的显示名称。
 * @param {string} fallback - 没有选中项时显示的默认标题。
 * @returns {void}
 */
function setDetailsTitle(element, value, fallback) {
  if (!element) return;
  const title = String(value || "").trim();
  element.textContent = title || fallback;
}

/**
 * 重置结构详情的内联重命名/删除确认状态。
 * @param {object} controls - 标题、输入框、下方操作区与按钮组节点。
 * @param {object|null} item - 当前选中的菜单项；为空时隐藏重命名按钮。
 * @returns {void}
 */
function resetRenameEditor(controls, item) {
  if (!controls || !controls.title || !controls.rename) return;
  controls.title.hidden = false;
  if (controls.itemSave) {
    controls.itemSave.hidden = !item;
    controls.itemSave.toggleAttribute("aria-hidden", !item);
    controls.itemSave.onclick = null;
  }
  if (controls.container) controls.container.hidden = !item;
  if (controls.input) {
    controls.input.hidden = true;
    controls.input.onkeydown = null;
  }
  if (controls.actions) controls.actions.hidden = true;
  if (controls.save) controls.save.onclick = null;
  if (controls.cancel) controls.cancel.onclick = null;
  if (controls.deleteActions) controls.deleteActions.hidden = true;
  if (controls.deleteConfirm) controls.deleteConfirm.onclick = null;
  if (controls.deleteCancel) controls.deleteCancel.onclick = null;
  controls.rename.hidden = !item;
  controls.rename.toggleAttribute("aria-hidden", !item);
  controls.rename.onclick = null;
  if (controls.remove) {
    controls.remove.hidden = !item;
    controls.remove.toggleAttribute("aria-hidden", !item);
    controls.remove.onclick = null;
  }
}

/**
 * 控制结构详情中的内联重命名与删除按钮。
 * @param {object} controls - 标题、输入框、下方操作区与按钮组节点。
 * @param {object|null} item - 当前选中的菜单项；为空时隐藏按钮。
 * @param {object} options - 保存、删除、重命名后需要触发的同步回调。
 * @returns {void}
 */
function setRenameButton(controls, item, options = {}) {
  resetRenameEditor(controls, item);
  if (!controls || !controls.rename || !item) return;
  controls.rename.onclick = () => startRenameEditor(controls, item, options);
  if (controls.remove) controls.remove.onclick = () => startDeleteConfirm(controls, item, options);
  if (controls.itemSave && typeof options.onItemSave === "function") {
    controls.itemSave.onclick = () => options.onItemSave();
  }
}

/**
 * 进入删除确认状态。
 * @param {object} controls - 详情区中的按钮组节点。
 * @param {object} item - 当前选中的菜单项或侧边栏项。
 * @param {object} options - 删除所需的 owner、itemIndex 与刷新回调。
 * @returns {void}
 */
function startDeleteConfirm(controls, item, options = {}) {
  if (!controls || !item || !controls.deleteActions) return;
  if (controls.container) controls.container.hidden = false;
  controls.rename.hidden = true;
  controls.rename.setAttribute("aria-hidden", "true");
  if (controls.remove) {
    controls.remove.hidden = true;
    controls.remove.setAttribute("aria-hidden", "true");
  }
  if (controls.actions) controls.actions.hidden = true;
  controls.deleteActions.hidden = false;
  if (controls.deleteConfirm) controls.deleteConfirm.onclick = () => deleteMenuItem(options);
  if (controls.deleteCancel) controls.deleteCancel.onclick = () => setRenameButton(controls, item, options);
}

/**
 * 删除当前选中的菜单项或侧边栏项。
 * @param {object} options - 删除所需的 owner、itemIndex 与刷新回调。
 * @returns {void}
 */
function deleteMenuItem(options = {}) {
  if (!options.owner || !Array.isArray(options.owner.items)) return;
  options.owner.items.splice(options.itemIndex, 1);
  if (options.onDelete) options.onDelete(options.itemIndex);
  if (options.onRender) options.onRender();
  if (typeof options.onDirty === "function") options.onDirty();
}

/**
 * 进入内联重命名模式。
 * @param {object} controls - 标题、输入框、下方操作区与按钮组节点。
 * @param {object} item - 当前正在编辑的菜单项或侧边栏项。
 * @param {object} options - 保存后需要触发的同步回调。
 * @returns {void}
 */
function startRenameEditor(controls, item, options = {}) {
  if (!controls || !controls.title || !controls.input || !controls.rename || !controls.actions) return;
  if (controls.container) controls.container.hidden = false;
  controls.input.value = itemTitle(item);
  controls.title.hidden = true;
  controls.input.hidden = false;
  controls.rename.hidden = true;
  controls.rename.setAttribute("aria-hidden", "true");
  if (controls.remove) {
    controls.remove.hidden = true;
    controls.remove.setAttribute("aria-hidden", "true");
  }
  if (controls.deleteActions) controls.deleteActions.hidden = true;
  controls.actions.hidden = false;
  if (controls.save) controls.save.onclick = () => saveRenameEditor(controls, item, options);
  if (controls.cancel) controls.cancel.onclick = () => cancelRenameEditor(controls, item, options);
  controls.input.onkeydown = (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      saveRenameEditor(controls, item, options);
    } else if (event.key === "Escape") {
      event.preventDefault();
      cancelRenameEditor(controls, item, options);
    }
  };
  window.requestAnimationFrame(() => {
    controls.input.focus();
    controls.input.select();
  });
}

/**
 * 保存标题栏内联重命名结果。
 * @param {object} controls - 标题、输入框与按钮组节点。
 * @param {object} item - 当前正在编辑的菜单项或侧边栏项。
 * @param {object} options - 保存后需要触发的同步回调；onRenameSave 用于持久化重命名结果。
 * @returns {void}
 */
function saveRenameEditor(controls, item, options = {}) {
  if (!controls || !controls.input) return;
  item.label = String(controls.input.value || "").trim();
  if (options.onTitleChange) options.onTitleChange(itemTitle(item));
  if (options.onChange) options.onChange();
  if (options.onRender) options.onRender();
  if (typeof options.onDirty === "function") options.onDirty();
  if (typeof options.onRenameSave === "function") options.onRenameSave();
}

/**
 * 取消标题栏内联重命名，恢复普通标题状态。
 * @param {object} controls - 标题、输入框与按钮组节点。
 * @param {object} item - 当前正在编辑的菜单项或侧边栏项。
 * @param {object} options - 重新绑定重命名、删除按钮时沿用的回调配置。
 * @returns {void}
 */
function cancelRenameEditor(controls, item, options = {}) {
  if (!controls || !controls.title) return;
  setDetailsTitle(controls.title, itemTitle(item), "");
  setRenameButton(controls, item, options);
}

function clampItemIndex(items, index) {
  if (!items.length) return -1;
  return Math.min(Math.max(index, 0), items.length - 1);
}

/**
 * 生成新建菜单项或侧边栏区块的默认名称。
 * @param {number} number - 当前同级列表中的序号。
 * @returns {string} 返回本地化后的默认名称。
 */
function newItemLabel(number) {
  return trFormat("menus.item.new_name", "New {number}", { number });
}

/**
 * 从默认名称中提取序号。
 * @param {string} label - 菜单项或侧边栏区块名称。
 * @returns {number} 能识别时返回序号，否则返回 0。
 */
function newItemNumberFromLabel(label) {
  const value = String(label || "").trim();
  for (const pattern of [/^新建(\d+)$/, /^New\s+(\d+)$/i]) {
    const match = value.match(pattern);
    if (match) return Number(match[1]);
  }
  const template = tr("menus.item.new_name", "New {number}");
  const marker = "{number}";
  const markerIndex = template.indexOf(marker);
  if (markerIndex < 0) return 0;
  const prefix = template.slice(0, markerIndex);
  const suffix = template.slice(markerIndex + marker.length);
  if (!value.startsWith(prefix) || !value.endsWith(suffix)) return 0;
  const numberText = value.slice(prefix.length, value.length - suffix.length).trim();
  return /^\d+$/.test(numberText) ? Number(numberText) : 0;
}

/**
 * 查找当前菜单里下一个可用的“新建N”序号。
 * @param {Array<{label: string}>} items - 当前菜单的菜单项列表。
 * @returns {number} 返回从 1 开始的最小可用序号。
 */
function nextNewMenuItemNumber(items) {
  const usedNumbers = new Set();
  items.forEach((item) => {
    const number = newItemNumberFromLabel(item.label);
    if (number > 0) usedNumbers.add(number);
  });
  let next = 1;
  while (usedNumbers.has(next)) next += 1;
  return next;
}

/**
 * 读取菜单项的目标配置。
 *
 * 兼容策略：
 * - 新 settings 结构从 item.main 读取 type/target/url。
 * - 旧 settings 结构继续从 item 顶层读取 type/target/url。
 *
 * @param {object} item - 后端返回的菜单项或侧边栏项。
 * @returns {object} 返回包含 type、target、url 的对象。
 */
function menuItemMain(item = {}) {
  if (item && typeof item.main === "object" && item.main !== null) return item.main;
  return item || {};
}

/**
 * 清洗菜单项类型，避免未知类型进入前端状态。
 * @param {string} value - 原始类型值。
 * @returns {string} 返回可识别的类型；空字符串表示未配置功能。
 */
function normalizeMenuItemType(value) {
  const type = String(value || "").trim().toLowerCase();
  if (type === "tagindex") return "tags";
  return ["", "page", "post", "tag", "url", ...systemFeatureTypes()].includes(type) ? type : "url";
}

/**
 * 把新旧 settings 菜单项转换成前端内部使用的扁平状态。
 * @param {object} item - 原始菜单项。
 * @returns {{type: string, label: string, target: string, url: string}} 返回前端可编辑状态。
 */
function normalizeMenuItem(item = {}) {
  const main = menuItemMain(item);
  const type = normalizeMenuItemType(main.type);
  return {
    type,
    label: String(item.label || ""),
    target: ["page", "post", "tag"].includes(type) ? String(main.target || "") : "",
    url: type === "url" ? String(main.url || "") : ""
  };
}

/**
 * 判断一组项目是否已经是新版侧边栏区块列表。
 * @param {Array<object>} items - 后端返回的侧边栏 items。
 * @returns {boolean} 包含任一侧边栏区块类型时返回 true。
 */
function hasSidebarBlocks(items) {
  return Array.isArray(items) && items.some((item) => sidebarBlockTypes().has(String(menuItemMain(item).type || "").trim().toLowerCase()));
}

/**
 * 把后端侧边栏区块转换成后台可编辑状态。
 * @param {object} item - 后端返回的侧边栏区块。
 * @returns {{type: string, label: string, items: Array}} 返回前端侧边栏区块状态。
 */
function normalizeSidebarBlock(item = {}) {
  const main = menuItemMain(item);
  const type = normalizeSidebarBlockType(main.type);
  return {
    type,
    label: String(item.label || ""),
    items: type === "custom" && Array.isArray(item.items) ? item.items.map(normalizeMenuItem) : []
  };
}

/**
 * 兼容旧版直接链接侧边栏：包装成一个自定义区域区块。
 * @param {object} sidebar - 后端返回的侧边栏。
 * @returns {Array<object>} 返回新版区块列表。
 */
function normalizeSidebarBlocks(sidebar = {}) {
  const items = Array.isArray(sidebar.items) ? sidebar.items : [];
  if (hasSidebarBlocks(items)) return items.map(normalizeSidebarBlock);
  if (!items.length) return [];
  return [{
    type: "custom",
    label: String(sidebar.name || tr("sidebar.section.default_title", "Custom Sidebar")),
    items: items.map(normalizeMenuItem)
  }];
}

/**
 * 把前端内部菜单项转换成 settings.json 的新结构。
 * @param {{type: string, label: string, target: string, url: string}} item - 前端菜单项状态。
 * @returns {{label: string, main: {type: string, target?: string, url?: string}}} 返回后端保存结构。
 */
function serializeMenuItem(item) {
  const type = normalizeMenuItemType(item.type);
  const main = {
    type
  };
  const target = String(item.target || "").trim();
  const url = String(item.url || "").trim();
  if (["page", "post", "tag"].includes(type) && target) main.target = target;
  if (type === "url" && url) main.url = url;
  return {
    label: String(item.label || "").trim(),
    main
  };
}

/**
 * 把前端侧边栏区块转换成 settings.json 结构。
 * @param {{type: string, label: string, items: Array}} block - 前端侧边栏区块状态。
 * @returns {{label: string, main: {type: string}, items?: Array}} 返回后端保存结构。
 */
function serializeSidebarBlock(block) {
  const type = normalizeSidebarBlockType(block.type);
  const result = {
    label: String(block.label || "").trim(),
    main: { type }
  };
  if (type === "custom") {
    result.items = (Array.isArray(block.items) ? block.items : []).map(serializeMenuItem);
  }
  return result;
}

function normalizeState(settings = {}) {
  const menus = Array.isArray(settings.menus) ? settings.menus : [];
  const normalizedMenus = menus.map((menu, index) => ({
    id: slugID(menu.id || menu.name, `menu-${index + 1}`),
    name: String(menu.name || menu.id || `Menu ${index + 1}`).trim(),
    items: Array.isArray(menu.items) ? menu.items.map(normalizeMenuItem) : []
  }));
  const sidebars = Array.isArray(settings.sidebars) ? settings.sidebars : [];
  const normalizedSidebars = sidebars.map((sidebar, index) => {
    const name = String(sidebar.name || `${tr("sidebar.section.default_name", "New Sidebar")} ${index + 1}`).trim();
    return {
      id: slugID(sidebar.id || name, `sidebar-${index + 1}`),
      name,
      items: normalizeSidebarBlocks(sidebar)
    };
  });
  return {
    menus: normalizedMenus,
    selectedIndex: normalizedMenus.length ? 0 : -1,
    selectedMenuItemIndex: normalizedMenus.length ? clampItemIndex(normalizedMenus[0].items, 0) : -1,
    sidebars: normalizedSidebars,
    selectedSidebarIndex: normalizedSidebars.length ? 0 : -1,
    selectedSidebarItemIndex: normalizedSidebars.length ? clampItemIndex(normalizedSidebars[0].items, 0) : -1,
    selectedSidebarCustomItemIndex: normalizedSidebars.length && normalizedSidebars[0].items[0] ? clampItemIndex(normalizedSidebars[0].items[0].items || [], 0) : -1
  };
}

const data = readInitialData();
const state = normalizeState(data.settings || {});
data.options = data.options || {};
const defaultThemeMenuID = String(data.default_menu_id || "default-menu");
const defaultSidebarID = String(data.default_sidebar_id || "default-sidebar");
let themeMenusSavedSnapshot = "";
let customSidebarSavedSnapshot = "";

/**
 * 判断菜单是否为受保护的默认菜单。
 *
 * 设计说明：
 * 默认菜单代表主题位置里的 “Default menu” 行为，只用于展示当前运行时默认结构；
 * 它不会写入 settings.json，因此前端禁止保存、删除和编辑。
 *
 * @param {{id: string}|null} menu - 当前菜单。
 * @returns {boolean} 如果菜单是默认菜单则返回 true。
 */
function isProtectedMenu(menu) {
  return Boolean(menu && String(menu.id || "") === defaultThemeMenuID);
}

/**
 * 判断侧边栏是否为受保护的默认侧边栏。
 *
 * 设计说明：
 * default-sidebar 和 default-menu 一样是运行时展示的默认结构，不写入
 * settings.json；用户需要新建侧边栏后再保存自己的配置。
 *
 * @param {{id: string}|null} sidebar - 当前侧边栏。
 * @returns {boolean} 如果侧边栏是默认侧边栏则返回 true。
 */
function isProtectedSidebar(sidebar) {
  return Boolean(sidebar && String(sidebar.id || "") === defaultSidebarID);
}

function replaceState(nextState) {
  state.menus = nextState.menus;
  state.selectedIndex = Math.min(Math.max(state.selectedIndex, 0), state.menus.length - 1);
  if (!state.menus.length) state.selectedIndex = -1;
  state.selectedMenuItemIndex = clampItemIndex(currentMenu() ? currentMenu().items : [], state.selectedMenuItemIndex);
  state.sidebars = nextState.sidebars;
  state.selectedSidebarIndex = Math.min(Math.max(state.selectedSidebarIndex, 0), state.sidebars.length - 1);
  if (!state.sidebars.length) state.selectedSidebarIndex = -1;
  state.selectedSidebarItemIndex = clampItemIndex(currentSidebarSection() ? currentSidebarSection().items : [], state.selectedSidebarItemIndex);
  state.selectedSidebarCustomItemIndex = clampItemIndex(currentSidebarCustomItems(), state.selectedSidebarCustomItemIndex);
}

function currentMenu() {
  if (state.selectedIndex < 0 || state.selectedIndex >= state.menus.length) return null;
  return state.menus[state.selectedIndex];
}

function currentSidebarSection() {
  if (state.selectedSidebarIndex < 0 || state.selectedSidebarIndex >= state.sidebars.length) return null;
  return state.sidebars[state.selectedSidebarIndex];
}

function currentMenuItem() {
  const menu = currentMenu();
  if (!menu || state.selectedMenuItemIndex < 0 || state.selectedMenuItemIndex >= menu.items.length) return null;
  return menu.items[state.selectedMenuItemIndex];
}

function currentSidebarItem() {
  const sidebar = currentSidebarSection();
  if (!sidebar || state.selectedSidebarItemIndex < 0 || state.selectedSidebarItemIndex >= sidebar.items.length) return null;
  return sidebar.items[state.selectedSidebarItemIndex];
}

function currentSidebarCustomItems() {
  const item = currentSidebarItem();
  if (!item || normalizeSidebarBlockType(item.type) !== "custom") return [];
  return Array.isArray(item.items) ? item.items : [];
}

function currentSidebarCustomItem() {
  const items = currentSidebarCustomItems();
  if (state.selectedSidebarCustomItemIndex < 0 || state.selectedSidebarCustomItemIndex >= items.length) return null;
  return items[state.selectedSidebarCustomItemIndex];
}

/**
 * 设置按钮禁用状态，并同步 is-disabled 类名。
 *
 * 设计说明：
 * 原生 disabled 可以阻止点击，但不同按钮样式不一定会自动变灰。
 * 这里同步 class，配合 CSS 让“不能保存/不能删除”等状态有一致视觉反馈。
 *
 * @param {HTMLButtonElement|null} button - 需要更新状态的按钮节点。
 * @param {boolean} disabled - true 表示禁用并显示灰色状态。
 * @returns {void}
 */
function setButtonDisabled(button, disabled) {
  if (!button) return;
  const isDisabled = Boolean(disabled);
  button.disabled = isDisabled;
  button.classList.toggle("is-disabled", isDisabled);
}

/**
 * 判断当前选中的菜单是否允许保存。
 *
 * 默认菜单是运行时生成的只读菜单，不写入 settings.json；
 * 没有选中菜单时也没有可保存对象，所以保存按钮应该禁用。
 *
 * @returns {boolean} 当前菜单可保存时返回 true。
 */
function canSaveCurrentThemeMenu() {
  const menu = currentMenu();
  return Boolean(menu && !isProtectedMenu(menu));
}

/**
 * 判断当前选中的侧边栏是否允许保存。
 *
 * default-sidebar 和 default-menu 一样是只读运行时结构，不应该保存到配置文件；
 * 没有选中侧边栏时也没有可保存对象。
 *
 * @returns {boolean} 当前侧边栏可保存时返回 true。
 */
function canSaveCurrentCustomSidebar() {
  const sidebar = currentSidebarSection();
  return Boolean(sidebar && !isProtectedSidebar(sidebar));
}

/**
 * 统一刷新主题菜单顶部操作按钮状态。
 *
 * 默认菜单是只读运行时菜单，所以不能保存、删除或新增菜单项；
 * 没有选中菜单时同样没有可操作对象。
 *
 * @returns {void}
 */
function updateThemeMenuActionStates() {
  const menu = currentMenu();
  const disabled = !menu || isProtectedMenu(menu);
  setButtonDisabled(saveThemeMenusButton, disabled);
  setButtonDisabled(deleteThemeMenuButton, disabled);
  setButtonDisabled(addThemeMenuItemButton, disabled);
}

/**
 * 统一刷新自定义侧边栏顶部和结构按钮状态。
 *
 * 逻辑和自定义菜单保持一致：不存在当前侧边栏时，保存、删除和新增结构项都禁用。
 *
 * @returns {void}
 */
function updateCustomSidebarActionStates() {
  const sidebar = currentSidebarSection();
  const disabled = !sidebar || isProtectedSidebar(sidebar);
  setButtonDisabled(saveCustomSidebarButton, disabled);
  setButtonDisabled(deleteSidebarSectionButton, disabled);
  setButtonDisabled(addSidebarSectionItemButton, disabled);
}

/**
 * 生成当前可持久化菜单的快照。
 *
 * 这里复用 payload()，因此 default-menu 会被排除；快照只代表真正会写入
 * settings.json 的菜单内容。
 *
 * @returns {string} 返回稳定 JSON 字符串，用于判断是否有未保存更改。
 */
function themeMenusSnapshot() {
  return JSON.stringify(payload().menus);
}

/**
 * 生成当前自定义侧边栏的保存快照。
 *
 * @returns {string} 返回稳定 JSON 字符串，用于判断侧边栏是否有未保存更改。
 */
function customSidebarSnapshot() {
  return JSON.stringify(payload().sidebars);
}

/**
 * 把当前菜单状态记录为“已保存”基线。
 * @returns {void}
 */
function resetThemeMenusSavedSnapshot() {
  themeMenusSavedSnapshot = themeMenusSnapshot();
}

/**
 * 把当前侧边栏状态记录为“已保存”基线。
 * @returns {void}
 */
function resetCustomSidebarSavedSnapshot() {
  customSidebarSavedSnapshot = customSidebarSnapshot();
}

/**
 * 根据保存快照刷新“未保存更改”状态。
 *
 * @returns {boolean} 如果当前可持久化菜单不同于已保存快照则返回 true。
 */
function refreshThemeMenusDirtyState() {
  const unsavedMessage = tr("settings.status.unsaved", "Unsaved changes");
  const dirty = themeMenusSnapshot() !== themeMenusSavedSnapshot;
  if (dirty) {
    setThemeMenuStatus(unsavedMessage);
  } else if (themeMenuStatus && themeMenuStatus.textContent === unsavedMessage) {
    setThemeMenuStatus(tr("settings.status.ready", "Ready"));
  }
  updateThemeMenuActionStates();
  return dirty;
}

/**
 * 根据保存快照刷新自定义侧边栏的“未保存更改”状态。
 *
 * @returns {boolean} 如果当前侧边栏不同于已保存快照则返回 true。
 */
function refreshCustomSidebarDirtyState() {
  const unsavedMessage = tr("settings.status.unsaved", "Unsaved changes");
  const dirty = customSidebarSnapshot() !== customSidebarSavedSnapshot;
  if (dirty) {
    setCustomSidebarStatus(unsavedMessage);
  } else if (customSidebarStatus && customSidebarStatus.textContent === unsavedMessage) {
    setCustomSidebarStatus(tr("settings.status.ready", "Ready"));
  }
  updateCustomSidebarActionStates();
  return dirty;
}

/**
 * 保存前提交当前正在编辑的菜单项重命名输入框。
 *
 * 设计说明：
 * 用户可能在右侧重命名输入框还没点小勾时直接点顶部“保存菜单”。
 * 此时输入框里的值还没有写入 item.label，必须先同步到内存状态再组装 payload。
 *
 * @returns {void}
 */
function commitActiveThemeMenuRename() {
  if (!themeMenuDetailsTitleInput || themeMenuDetailsTitleInput.hidden) return;
  const item = currentMenuItem();
  if (!item) return;
  item.label = String(themeMenuDetailsTitleInput.value || "").trim();
  renderMenuItemList();
  renderMenuStructure();
  refreshThemeMenusDirtyState();
}

/**
 * 保存前提交当前正在编辑的侧边栏项重命名输入框。
 *
 * 侧边栏复用菜单结构编辑器的重命名交互；顶部保存时同样需要先把输入框内容
 * 同步到内存状态，避免用户输入后直接保存丢失当前标题。
 *
 * @returns {void}
 */
function commitActiveCustomSidebarRename() {
  if (!sidebarSectionDetailsTitleInput || sidebarSectionDetailsTitleInput.hidden) return;
  const item = currentSidebarItem();
  if (!item) return;
  item.label = String(sidebarSectionDetailsTitleInput.value || "").trim();
  renderSidebarItemList();
  renderSidebarStructure();
  refreshCustomSidebarDirtyState();
}

/**
 * 同步可编辑下拉框箭头的展开状态。
 * @param {HTMLButtonElement|null} toggle - 负责展开列表的箭头按钮。
 * @param {boolean} open - true 表示列表已展开，false 表示列表已收起。
 * @returns {void}
 */
function setEditableSelectorOpen(toggle, open) {
  if (!toggle) return;
  toggle.setAttribute("aria-expanded", open ? "true" : "false");
  toggle.textContent = open ? "▴" : "▾";
}

/**
 * 关闭可编辑下拉框列表。
 * @param {HTMLElement|null} list - 下拉列表容器。
 * @param {HTMLButtonElement|null} toggle - 负责展开列表的箭头按钮。
 * @returns {void}
 */
function closeEditableSelector(list, toggle) {
  if (list) list.hidden = true;
  setEditableSelectorOpen(toggle, false);
}

/**
 * 渲染可编辑下拉框。输入区用于改名，箭头列表用于切换当前编辑对象。
 * @param {object} config - 组合框配置。
 * @param {HTMLInputElement|null} config.input - 可编辑输入框。
 * @param {HTMLButtonElement|null} config.toggle - 箭头展开按钮。
 * @param {HTMLElement|null} config.list - 下拉列表容器。
 * @param {Array<{id: string, name: string}>} config.items - 可选择的菜单或侧边栏数组。
 * @param {number} config.selectedIndex - 当前选中索引。
 * @param {string} config.emptyLabel - 没有项目时显示的占位文案。
 * @param {(index: number) => void} config.onSelect - 点击列表项后的选择回调。
 * @param {boolean} [config.syncInput=true] - 是否同步输入框文字，输入过程中会关闭以保留光标。
 * @returns {void}
 */
function renderEditableSelector(config) {
  const { input, toggle, list, items, selectedIndex, emptyLabel, onSelect, syncInput = true } = config;
  if (!input) return;
  const selected = selectedIndex >= 0 && selectedIndex < items.length ? items[selectedIndex] : null;
  input.disabled = !items.length;
  if (toggle) toggle.disabled = !items.length;
  if (syncInput) input.value = selected ? selected.name || selected.id : emptyLabel;
  if (!list) return;
  list.innerHTML = "";
  if (!items.length) {
    closeEditableSelector(list, toggle);
    return;
  }
  items.forEach((item, index) => {
    const option = document.createElement("button");
    option.type = "button";
    option.className = "editable-selector__option";
    if (index === selectedIndex) option.classList.add("is-active");
    option.setAttribute("role", "option");
    option.setAttribute("aria-selected", index === selectedIndex ? "true" : "false");
    option.textContent = item.name || item.id;
    option.addEventListener("click", () => {
      closeEditableSelector(list, toggle);
      onSelect(index);
    });
    list.appendChild(option);
  });
}

function renderMenuSelector(options = {}) {
  const selectedMenu = currentMenu();
  renderEditableSelector({
    input: menuSelector,
    toggle: menuSelectorToggle,
    list: menuSelectorList,
    items: state.menus,
    selectedIndex: state.selectedIndex,
    emptyLabel: tr("menus.menu.empty", "No custom menus yet."),
    onSelect: selectMenu,
    syncInput: options.syncInput !== false
  });
  if (menuSelector) menuSelector.readOnly = isProtectedMenu(selectedMenu);
}

function renderSidebarSectionSelector(options = {}) {
  const selectedSidebar = currentSidebarSection();
  renderEditableSelector({
    input: sidebarSectionSelector,
    toggle: sidebarSectionSelectorToggle,
    list: sidebarSectionSelectorList,
    items: state.sidebars,
    selectedIndex: state.selectedSidebarIndex,
    emptyLabel: tr("sidebar.section.empty", "No custom sidebars yet."),
    onSelect: selectSidebarSection,
    syncInput: options.syncInput !== false
  });
  if (sidebarSectionSelector) sidebarSectionSelector.readOnly = isProtectedSidebar(selectedSidebar);
}

/**
 * 渲染左侧项目列表，点击后切换中间的结构详情。
 * @param {HTMLElement|null} container - 左侧项目列表容器。
 * @param {Array<{type: string, label: string, target: string, url: string}>} items - 菜单项或侧边栏项数组。
 * @param {number} selectedIndex - 当前选中索引。
 * @param {string} emptyLabel - 没有项目时显示的占位文案。
 * @param {(index: number) => void} onSelect - 点击列表项后的选择回调。
 * @param {(index: number, direction: string) => void} onMove - 点击上下箭头后的排序回调。
 * @returns {void}
 */
function renderItemList(container, items, selectedIndex, emptyLabel, onSelect, onMove, options = {}) {
  if (!container) return;
  container.innerHTML = "";
  if (!items.length) {
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = emptyLabel;
    container.appendChild(empty);
    updateItemListOverflowState(container);
    return;
  }
  items.forEach((item, index) => {
    const row = document.createElement("div");
    row.className = "menu-item-list__row";
    if (index === selectedIndex) row.classList.add("is-active");
    const button = document.createElement("button");
    button.type = "button";
    button.className = "menu-item-list__button";
    button.setAttribute("aria-current", index === selectedIndex ? "true" : "false");
    button.textContent = itemTitle(item);
    button.addEventListener("click", () => onSelect(index));
    const controls = document.createElement("div");
    controls.className = "menu-item-sort-controls";
    controls.setAttribute("aria-label", tr("menus.item.order", "Menu item ordering"));
    controls.append(
      itemSortButton("up", options.readOnly || index <= 0, () => onMove(index, "up")),
      itemSortButton("down", options.readOnly || index >= items.length - 1, () => onMove(index, "down"))
    );
    row.append(button, controls);
    container.appendChild(row);
  });
  updateItemListOverflowState(container);
}

/**
 * 根据左侧结构列表是否真的出现纵向滚动条，切换滚动状态类。
 * @param {HTMLElement|null} container - 菜单项或侧边栏项的滚动列表容器。
 * @returns {void}
 */
function updateItemListOverflowState(container) {
  if (!container) return;
  const hasVerticalScrollbar = container.scrollHeight > container.clientHeight + 1;
  container.classList.toggle("menu-item-list--scrolling", hasVerticalScrollbar);
}

function renderMenuItemList() {
  const menu = currentMenu();
  const emptyLabel = menu ? tr("menus.structure.empty", "This menu has no items.") : tr("menus.menu.empty", "No custom menus yet.");
  renderItemList(themeMenuItemList, menu ? menu.items : [], state.selectedMenuItemIndex, emptyLabel, selectMenuItem, moveMenuItem, {
    readOnly: isProtectedMenu(menu)
  });
}

function renderSidebarItemList() {
  const sidebar = currentSidebarSection();
  const emptyLabel = sidebar ? tr("sidebar.structure.empty", "This sidebar has no items.") : tr("sidebar.section.empty", "No custom sidebars yet.");
  renderItemList(sidebarSectionItemList, sidebar ? sidebar.items : [], state.selectedSidebarItemIndex, emptyLabel, selectSidebarItem, moveSidebarItem, {
    readOnly: isProtectedSidebar(sidebar)
  });
}

function itemSortButton(direction, disabled, onClick) {
  const label = tr(`menus.item.move_${direction}`, direction === "up" ? "Move Up" : "Move Down");
  const button = document.createElement("button");
  button.type = "button";
  button.className = `ui-button plugin-arrow-button plugin-arrow-button--${direction}`;
  button.title = label;
  button.setAttribute("aria-label", label);
  button.disabled = disabled;
  button.addEventListener("click", (event) => {
    event.stopPropagation();
    if (button.disabled) return;
    onClick();
  });
  return button;
}

function renderMenuStructure() {
  const menu = currentMenu();
  const fallbackTitle = tr("menus.structure.details_title", "Structure Details");
  const readOnly = isProtectedMenu(menu);
  updateThemeMenuActionStates();
  if (!themeMenuList) return;
  themeMenuList.innerHTML = "";
  if (!menu) {
    setDetailsTitle(themeMenuDetailsTitle, "", fallbackTitle);
    setRenameButton(themeMenuRenameControls, null);
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = tr("menus.menu.empty", "No custom menus yet.");
    themeMenuList.appendChild(empty);
    return;
  }
  if (!menu.items.length) {
    setDetailsTitle(themeMenuDetailsTitle, "", fallbackTitle);
    setRenameButton(themeMenuRenameControls, null);
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = tr("menus.structure.empty", "This menu has no items.");
    themeMenuList.appendChild(empty);
    return;
  }
  const item = currentMenuItem();
  if (!item) {
    setDetailsTitle(themeMenuDetailsTitle, "", fallbackTitle);
    setRenameButton(themeMenuRenameControls, null);
    return;
  }
  setDetailsTitle(themeMenuDetailsTitle, itemTitle(item), fallbackTitle);
  if (readOnly) {
    setRenameButton(themeMenuRenameControls, null);
  } else {
    setRenameButton(themeMenuRenameControls, item, {
      owner: menu,
      itemIndex: state.selectedMenuItemIndex,
      onChange: renderMenuItemList,
      onTitleChange: (title) => setDetailsTitle(themeMenuDetailsTitle, title, fallbackTitle),
      onItemSave: refreshThemeMenusDirtyState,
      onRenameSave: refreshThemeMenusDirtyState,
      onDirty: refreshThemeMenusDirtyState,
      onRender: () => {
        renderMenuItemList();
        renderMenuStructure();
      },
      onDelete: (deletedIndex) => {
        state.selectedMenuItemIndex = clampItemIndex(menu.items, deletedIndex);
      }
    });
  }
  themeMenuList.appendChild(renderItemDetailsEditor(item, {
    onChange: renderMenuItemList,
    onDirty: refreshThemeMenusDirtyState,
    onTitleChange: (title) => setDetailsTitle(themeMenuDetailsTitle, title, fallbackTitle),
    onRender: () => {
      renderMenuItemList();
      renderMenuStructure();
    }
  }, { readOnly }));
}

/**
 * 渲染详情区里的真实菜单项编辑器。
 *
 * 设计说明：
 * - “系统功能页”直接保存为 main.type，例如 home、archive、tags、search、admin。
 * - “自定义页面”对应 page/post/tag 三种内容目标，会写入 type + target。
 * - “自定义链接”对应 url，会写入 type + url。
 *
 * @param {{type: string, label: string, target: string, url: string}} item - 当前选中的菜单项或侧边栏项。
 * @param {object} callbacks - 详情变化后需要触发的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时只展示，不绑定修改事件。
 * @returns {HTMLElement} 返回可挂载到结构详情区的编辑器节点。
 */
function renderItemDetailsEditor(item, callbacks = {}, options = {}) {
  const row = document.createElement("article");
  row.className = "menu-structure-item";
  const fields = document.createElement("div");
  fields.className = "menu-structure-item__fields";
  fields.append(renderItemDetailsControls(item, callbacks, options));
  row.append(fields);
  return row;
}

/**
 * 渲染菜单项详情控件片段，供自定义菜单和侧边栏自定义区域复用。
 * @param {{type: string, label: string, target: string, url: string}} item - 当前菜单项。
 * @param {object} callbacks - 状态变化后的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时禁用编辑控件。
 * @returns {DocumentFragment} 返回可直接挂载的详情控件片段。
 */
function renderItemDetailsControls(item, callbacks = {}, options = {}) {
  const readOnly = Boolean(options.readOnly);
  const fragment = document.createDocumentFragment();
  const modeSelect = document.createElement("select");
  modeSelect.className = "ui-input";
  modeSelect.setAttribute("aria-label", tr("menus.item.mode.label", "Menu item type"));
  [
    ["", tr("menus.item.mode.none", "No function")],
    ["feature", tr("menus.item.mode.feature", "System page")],
    ["content", tr("menus.item.mode.content", "Content item")],
    ["link", tr("menus.item.mode.link", "Custom link")]
  ].forEach(([value, label]) => {
    modeSelect.appendChild(new Option(label, value));
  });
  modeSelect.value = currentItemMode(item);
  modeSelect.disabled = readOnly;
  const detailArea = document.createElement("div");
  detailArea.className = "menu-structure-blank-area menu-structure-detail-area";
  renderDetailsForMode(detailArea, item, callbacks, { readOnly });
  if (!readOnly) {
    modeSelect.addEventListener("change", () => {
      applyItemMode(item, modeSelect.value);
      notifyItemChanged(item, callbacks, { rerender: true });
    });
  }
  fragment.append(modeSelect, detailArea);
  return fragment;
}

/**
 * 根据当前菜单项类型渲染下方详情控件。
 * @param {HTMLElement} container - 详情控件容器。
 * @param {{type: string, label: string, target: string, url: string}} item - 当前菜单项。
 * @param {object} callbacks - 状态变化后的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时禁用所有编辑控件。
 * @returns {void}
 */
function renderDetailsForMode(container, item, callbacks = {}, options = {}) {
  const mode = currentItemMode(item);
  container.innerHTML = "";
  if (mode === "feature") {
    container.appendChild(renderFeatureSelect(item, callbacks, options));
  } else if (mode === "content") {
    container.appendChild(renderContentTypeSelect(item, callbacks, options));
    container.appendChild(renderContentTargetList(item, callbacks, options));
  } else if (mode === "link") {
    container.appendChild(renderCustomURLInput(item, callbacks, options));
  } else {
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = tr("menus.item.blank_hint", "Choose a type to configure this item.");
    container.appendChild(empty);
  }
}

/**
 * 渲染系统功能页选择框。
 * @param {object} item - 当前菜单项。
 * @param {object} callbacks - 状态变化后的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时禁用选择框。
 * @returns {HTMLSelectElement} 返回系统功能页下拉框。
 */
function renderFeatureSelect(item, callbacks = {}, options = {}) {
  const select = document.createElement("select");
  select.className = "ui-input";
  select.setAttribute("aria-label", tr("menus.item.mode.feature", "System page"));
  select.disabled = Boolean(options.readOnly);
  const currentFeature = featureByType(item.type) || featureByURL(item.url) || systemFeatureOptions[0];
  systemFeatureOptions.forEach((feature) => {
    select.appendChild(new Option(feature.label, feature.value));
  });
  select.value = currentFeature.value;
  if (!options.readOnly) {
    select.addEventListener("change", () => {
      applyFeatureSelection(item, select.value);
      notifyItemChanged(item, callbacks);
    });
  }
  return select;
}

/**
 * 渲染内容类型选择框。
 * @param {object} item - 当前菜单项。
 * @param {object} callbacks - 状态变化后的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时禁用选择框。
 * @returns {HTMLSelectElement} 返回内容类型 select。
 */
function renderContentTypeSelect(item, callbacks = {}, options = {}) {
  const select = document.createElement("select");
  select.className = "ui-input";
  select.setAttribute("aria-label", tr("menus.item.content_type", "Content type"));
  select.disabled = Boolean(options.readOnly);
  [
    ["page", tr("menus.item.type.page", "Page")],
    ["post", tr("menus.item.type.post", "Post")],
    ["tag", tr("menus.item.type.tag", "Tag")]
  ].forEach(([value, label]) => {
    select.appendChild(new Option(label, value));
  });
  select.value = ["page", "post", "tag"].includes(item.type) ? item.type : firstAvailableContentType();
  if (!options.readOnly) {
    select.addEventListener("change", () => {
      applyContentSelection(item, select.value, firstOptionSlug(select.value));
      notifyItemChanged(item, callbacks, { rerender: true });
    });
  }
  return select;
}

/**
 * 渲染具体内容目标列表。
 * @param {object} item - 当前菜单项。
 * @param {object} callbacks - 状态变化后的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时禁用目标按钮。
 * @returns {HTMLElement} 返回可点击的内容目标列表。
 */
function renderContentTargetList(item, callbacks = {}, options = {}) {
  const list = document.createElement("div");
  list.className = "menu-detail-option-list";
  const contentOptions = optionsForType(item.type);
  if (!contentOptions.length) {
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = tr("menus.source.empty", "No items available.");
    list.appendChild(empty);
    return list;
  }
  const selectedSlug = contentOptions.some((option) => option.slug === item.target) ? item.target : contentOptions[0].slug;
  if (selectedSlug !== item.target && !options.readOnly) {
    applyContentSelection(item, item.type, selectedSlug);
  }
  contentOptions.forEach((option) => {
    list.appendChild(renderDetailOption({
      title: option.title,
      meta: option.slug,
      selected: option.slug === selectedSlug,
      disabled: Boolean(options.readOnly),
      onSelect: () => {
        applyContentSelection(item, item.type, option.slug);
        notifyItemChanged(item, callbacks, { rerender: true });
      }
    }));
  });
  return list;
}

/**
 * 渲染详情区里的单个可点击选项。
 * @param {{title: string, meta: string, selected: boolean, onSelect: Function}} option - 列表项配置。
 * @returns {HTMLButtonElement} 返回列表按钮。
 */
function renderDetailOption(option) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "menu-detail-option";
  button.disabled = Boolean(option.disabled);
  if (option.selected) button.classList.add("is-selected");
  const title = document.createElement("span");
  title.className = "menu-detail-option__title";
  title.textContent = option.title || option.meta;
  button.append(title);
  if (!option.disabled) {
    button.addEventListener("click", () => {
      if (typeof option.onSelect === "function") option.onSelect();
    });
  }
  return button;
}

/**
 * 渲染自定义链接 URL 输入框。
 * @param {object} item - 当前菜单项。
 * @param {object} callbacks - 状态变化后的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时只读展示 URL。
 * @returns {HTMLInputElement} 返回 URL 输入框。
 */
function renderCustomURLInput(item, callbacks = {}, options = {}) {
  const input = document.createElement("input");
  input.className = "ui-input";
  input.type = "text";
  input.placeholder = tr("menus.source.custom_url", "URL");
  input.value = item.url || "";
  input.setAttribute("aria-label", tr("menus.source.custom_url", "URL"));
  input.readOnly = Boolean(options.readOnly);
  if (!options.readOnly) {
    input.addEventListener("input", () => {
      preserveVisibleLabel(item);
      item.type = "url";
      item.target = "";
      item.url = String(input.value || "").trim();
      notifyItemChanged(item, callbacks);
    });
  }
  return input;
}

/**
 * 切换菜单项模式，并写入可保存的数据结构。
 * @param {object} item - 当前菜单项。
 * @param {string} mode - feature、content、link 或空字符串。
 * @returns {void}
 */
function applyItemMode(item, mode) {
  if (!item) return;
  preserveVisibleLabel(item);
  if (mode === "feature") {
    applyFeatureSelection(item, (featureByType(item.type) || featureByURL(item.url) || systemFeatureOptions[0]).value);
  } else if (mode === "content") {
    const contentType = ["page", "post", "tag"].includes(item.type) ? item.type : firstAvailableContentType();
    applyContentSelection(item, contentType, item.target || firstOptionSlug(contentType));
  } else if (mode === "link") {
    item.type = "url";
    item.target = "";
    if (!item.url || featureByURL(item.url) || systemFeatureTypes().has(item.type)) item.url = "";
  } else {
    item.type = "";
    item.target = "";
    item.url = "";
  }
}

/**
 * 写入系统功能页选择结果。
 * @param {object} item - 当前菜单项。
 * @param {string} value - 系统功能页 value。
 * @returns {void}
 */
function applyFeatureSelection(item, value) {
  preserveVisibleLabel(item);
  const feature = featureByValue(value);
  item.type = feature.value;
  item.target = "";
  item.url = "";
}

/**
 * 写入页面、文章或标签目标选择结果。
 * @param {object} item - 当前菜单项。
 * @param {string} type - page、post 或 tag。
 * @param {string} target - 目标内容 slug。
 * @returns {void}
 */
function applyContentSelection(item, type, target) {
  preserveVisibleLabel(item);
  item.type = ["page", "post", "tag"].includes(type) ? type : "page";
  item.target = target || firstOptionSlug(item.type);
  item.url = "";
}

/**
 * 通知外层刷新当前项标题、结构列表或整个详情区。
 * @param {object} item - 当前菜单项。
 * @param {object} callbacks - 外层渲染回调。
 * @param {{rerender?: boolean}} options - 是否需要重建详情区。
 * @returns {void}
 */
function notifyItemChanged(item, callbacks = {}, options = {}) {
  if (callbacks.onTitleChange) callbacks.onTitleChange(itemTitle(item));
  if (callbacks.onChange) callbacks.onChange();
  if (options.rerender && callbacks.onRender) callbacks.onRender();
  if (typeof callbacks.onDirty === "function") callbacks.onDirty();
}

function renderSidebarStructure() {
  const sidebar = currentSidebarSection();
  const fallbackTitle = tr("sidebar.structure.details_title", "Structure Details");
  const readOnly = isProtectedSidebar(sidebar);
  updateCustomSidebarActionStates();
  if (!sidebarSectionList) return;
  sidebarSectionList.innerHTML = "";
  if (!sidebar) {
    setDetailsTitle(sidebarSectionDetailsTitle, "", fallbackTitle);
    setRenameButton(sidebarSectionRenameControls, null);
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = tr("sidebar.section.empty", "No custom sidebars yet.");
    sidebarSectionList.appendChild(empty);
    return;
  }
  if (!sidebar.items.length) {
    setDetailsTitle(sidebarSectionDetailsTitle, "", fallbackTitle);
    setRenameButton(sidebarSectionRenameControls, null);
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = tr("sidebar.structure.empty", "This sidebar has no items.");
    sidebarSectionList.appendChild(empty);
    return;
  }
  const item = currentSidebarItem();
  if (!item) {
    setDetailsTitle(sidebarSectionDetailsTitle, "", fallbackTitle);
    setRenameButton(sidebarSectionRenameControls, null);
    return;
  }
  setDetailsTitle(sidebarSectionDetailsTitle, itemTitle(item), fallbackTitle);
  if (readOnly) {
    setRenameButton(sidebarSectionRenameControls, null);
  } else {
    setRenameButton(sidebarSectionRenameControls, item, {
      owner: sidebar,
      itemIndex: state.selectedSidebarItemIndex,
      onChange: renderSidebarItemList,
      onTitleChange: (title) => setDetailsTitle(sidebarSectionDetailsTitle, title, fallbackTitle),
      onItemSave: refreshCustomSidebarDirtyState,
      onRenameSave: refreshCustomSidebarDirtyState,
      onDirty: refreshCustomSidebarDirtyState,
      onRender: () => {
        renderSidebarItemList();
        renderSidebarStructure();
      },
      onDelete: (deletedIndex) => {
        state.selectedSidebarItemIndex = clampItemIndex(sidebar.items, deletedIndex);
      }
    });
  }
  sidebarSectionList.appendChild(renderSidebarBlockDetailsEditor(item, {
    onChange: renderSidebarItemList,
    onTitleChange: (title) => setDetailsTitle(sidebarSectionDetailsTitle, title, fallbackTitle),
    onDirty: refreshCustomSidebarDirtyState,
    onRender: () => {
      renderSidebarItemList();
      renderSidebarStructure();
    }
  }, { readOnly }));
}

/**
 * 渲染侧边栏区块详情编辑器。
 * @param {{type: string, label: string, items: Array}} block - 当前选中的侧边栏区块。
 * @param {object} callbacks - 区块或子项目变化后的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时按默认侧边栏展示，不允许修改。
 * @returns {HTMLElement} 返回详情编辑器节点。
 */
function renderSidebarBlockDetailsEditor(block, callbacks = {}, options = {}) {
  const row = document.createElement("article");
  row.className = "menu-structure-item";
  const fields = document.createElement("div");
  fields.className = "menu-structure-item__fields";
  const detailArea = document.createElement("div");
  detailArea.className = "menu-structure-blank-area menu-structure-detail-area";
  if (sidebarBlockMode(block) === "custom") {
    detailArea.appendChild(renderCustomSidebarBlockEditor(block, callbacks, options));
  } else if (sidebarBlockMode(block) === "auto") {
    detailArea.appendChild(renderSidebarAutoBlockEditor(block, callbacks, options));
  } else {
    detailArea.appendChild(renderSidebarNoFunctionEditor());
  }
  fields.append(renderSidebarBlockModeSelect(block, callbacks, options), detailArea);
  row.append(fields);
  return row;
}

/**
 * 渲染侧边栏结构项的无功能空状态。
 * @returns {HTMLParagraphElement} 返回和菜单无功能状态一致的提示节点。
 */
function renderSidebarNoFunctionEditor() {
  const empty = document.createElement("p");
  empty.className = "theme-settings-empty";
  empty.textContent = tr("menus.item.blank_hint", "Choose a type to configure this item.");
  return empty;
}

/**
 * 渲染侧边栏区块模式选择框。
 * @param {object} block - 当前侧边栏区块。
 * @param {object} callbacks - 模式变化后的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时禁用选择框。
 * @returns {HTMLSelectElement} 返回区块模式选择框。
 */
function renderSidebarBlockModeSelect(block, callbacks = {}, options = {}) {
  const select = document.createElement("select");
  select.className = "ui-input";
  select.setAttribute("aria-label", tr("sidebar.block.mode.label", "Sidebar block mode"));
  select.appendChild(new Option(tr("sidebar.type.none", "No function"), ""));
  select.appendChild(new Option(tr("sidebar.type.auto", "Automatic area"), "auto"));
  select.appendChild(new Option(tr("sidebar.type.custom", "Custom Area"), "custom"));
  select.value = sidebarBlockMode(block);
  select.disabled = Boolean(options.readOnly);
  if (options.readOnly) return select;
  select.addEventListener("change", () => {
    if (select.value === "custom") {
      block.type = "custom";
    } else if (select.value === "auto") {
      block.type = sidebarBlockTypeOptions[0].value;
      block.items = [];
    } else {
      block.type = "";
      block.items = [];
    }
    if (!Array.isArray(block.items)) block.items = [];
    state.selectedSidebarCustomItemIndex = clampItemIndex(currentSidebarCustomItems(), state.selectedSidebarCustomItemIndex);
    if (callbacks.onTitleChange) callbacks.onTitleChange(itemTitle(block));
    if (callbacks.onRender) callbacks.onRender();
    if (typeof callbacks.onDirty === "function") callbacks.onDirty();
  });
  return select;
}

/**
 * 渲染自动生成区块的具体类型选择。
 * @param {object} block - 当前侧边栏区块。
 * @param {object} callbacks - 类型变化后的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时禁用选择框。
 * @returns {HTMLElement} 返回自动区块详情节点。
 */
function renderSidebarAutoBlockEditor(block, callbacks = {}, options = {}) {
  const select = document.createElement("select");
  select.className = "ui-input";
  select.setAttribute("aria-label", tr("sidebar.block.auto_type", "Automatic block type"));
  sidebarBlockTypeOptions.forEach((option) => {
    select.appendChild(new Option(option.label, option.value));
  });
  select.value = sidebarBlockTypeOptions.some((option) => option.value === normalizeSidebarBlockType(block.type))
    ? normalizeSidebarBlockType(block.type)
    : sidebarBlockTypeOptions[0].value;
  select.disabled = Boolean(options.readOnly);
  if (!options.readOnly) {
    select.addEventListener("change", () => {
      block.type = normalizeSidebarBlockType(select.value);
      block.items = [];
      if (callbacks.onTitleChange) callbacks.onTitleChange(itemTitle(block));
      if (callbacks.onRender) callbacks.onRender();
      if (typeof callbacks.onDirty === "function") callbacks.onDirty();
    });
  }
  return select;
}

/**
 * 渲染自定义区域内部的链接编辑器。
 * @param {{items: Array}} block - 自定义区域区块。
 * @param {object} callbacks - 子项目变化后的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时禁用新增和编辑。
 * @returns {HTMLElement} 返回自定义链接编辑区。
 */
function renderCustomSidebarBlockEditor(block, callbacks = {}, options = {}) {
  const editor = document.createElement("div");
  editor.className = "sidebar-custom-editor";
  const actions = document.createElement("div");
  actions.className = "sidebar-custom-editor__actions";
  const addButton = document.createElement("button");
  addButton.type = "button";
  addButton.className = "ui-button";
  addButton.textContent = tr("sidebar.custom.add_item", "Add custom item");
  addButton.disabled = Boolean(options.readOnly);
  if (!options.readOnly) {
    addButton.addEventListener("click", () => {
      if (!Array.isArray(block.items)) block.items = [];
      block.items.push(createBlankMenuItem(block.items));
      state.selectedSidebarCustomItemIndex = block.items.length - 1;
      if (callbacks.onRender) callbacks.onRender();
      if (typeof callbacks.onDirty === "function") callbacks.onDirty();
    });
  }
  actions.appendChild(addButton);
  editor.appendChild(actions);

  const items = Array.isArray(block.items) ? block.items : [];
  const list = document.createElement("div");
  list.className = "menu-detail-option-list sidebar-custom-editor__list";
  if (!items.length) {
    const empty = document.createElement("p");
    empty.className = "theme-settings-empty";
    empty.textContent = tr("sidebar.custom.empty", "This custom area has no items.");
    list.appendChild(empty);
  } else {
    items.forEach((child, index) => {
      list.appendChild(renderDetailOption({
        title: itemTitle(child) || trFormat("menus.item.numbered", "Item {number}", { number: index + 1 }),
        meta: "",
        selected: index === state.selectedSidebarCustomItemIndex,
        disabled: Boolean(options.readOnly),
        onSelect: () => {
          state.selectedSidebarCustomItemIndex = index;
          if (callbacks.onRender) callbacks.onRender();
        }
      }));
    });
  }
  editor.appendChild(list);

  const child = currentSidebarCustomItem();
  if (child) {
    editor.appendChild(renderCustomSidebarChildEditor(block, child, callbacks, options));
  }
  return editor;
}

/**
 * 渲染自定义区域中单个链接项的编辑器。
 * @param {{items: Array}} block - 当前自定义区域区块。
 * @param {object} child - 当前选中的子链接项。
 * @param {object} callbacks - 子项目变化后的刷新回调。
 * @param {{readOnly?: boolean}} options - readOnly 为 true 时禁用输入和删除。
 * @returns {HTMLElement} 返回子链接编辑器。
 */
function renderCustomSidebarChildEditor(block, child, callbacks = {}, options = {}) {
  const wrapper = document.createElement("div");
  wrapper.className = "sidebar-custom-child-editor";
  const tools = document.createElement("div");
  tools.className = "sidebar-custom-child-editor__tools";

  const nameInput = document.createElement("input");
  nameInput.className = "ui-input";
  nameInput.type = "text";
  nameInput.placeholder = tr("menus.item.display_name", "Display name");
  nameInput.value = itemTitle(child);
  nameInput.readOnly = Boolean(options.readOnly);
  if (!options.readOnly) {
    nameInput.addEventListener("input", () => {
      child.label = String(nameInput.value || "").trim();
      if (typeof callbacks.onDirty === "function") callbacks.onDirty();
    });
  }

  const deleteButton = document.createElement("button");
  deleteButton.type = "button";
  deleteButton.className = "ui-button";
  deleteButton.textContent = tr("menus.item.delete_item", "Delete item");
  deleteButton.disabled = Boolean(options.readOnly);
  if (!options.readOnly) {
    deleteButton.addEventListener("click", () => {
      const index = state.selectedSidebarCustomItemIndex;
      if (index < 0 || index >= block.items.length) return;
      block.items.splice(index, 1);
      state.selectedSidebarCustomItemIndex = clampItemIndex(block.items, index);
      if (callbacks.onRender) callbacks.onRender();
      if (typeof callbacks.onDirty === "function") callbacks.onDirty();
    });
  }

  tools.append(nameInput, deleteButton);
  wrapper.appendChild(tools);
  wrapper.append(renderItemDetailsControls(child, {
    onChange: callbacks.onRender,
    onTitleChange: () => {},
    onDirty: callbacks.onDirty,
    onRender: callbacks.onRender
  }, options));
  return wrapper;
}

function selectMenu(index) {
  if (state.menus.length === 0) state.selectedIndex = -1;
  else state.selectedIndex = Math.min(Math.max(index, 0), state.menus.length - 1);
  state.selectedMenuItemIndex = clampItemIndex(currentMenu() ? currentMenu().items : [], 0);
  renderAll();
}

function selectSidebarSection(index) {
  if (state.sidebars.length === 0) state.selectedSidebarIndex = -1;
  else state.selectedSidebarIndex = Math.min(Math.max(index, 0), state.sidebars.length - 1);
  state.selectedSidebarItemIndex = clampItemIndex(currentSidebarSection() ? currentSidebarSection().items : [], 0);
  state.selectedSidebarCustomItemIndex = clampItemIndex(currentSidebarCustomItems(), 0);
  renderAll();
}

function selectMenuItem(index) {
  const menu = currentMenu();
  state.selectedMenuItemIndex = clampItemIndex(menu ? menu.items : [], index);
  renderMenuItemList();
  renderMenuStructure();
}

function selectSidebarItem(index) {
  const sidebar = currentSidebarSection();
  state.selectedSidebarItemIndex = clampItemIndex(sidebar ? sidebar.items : [], index);
  state.selectedSidebarCustomItemIndex = clampItemIndex(currentSidebarCustomItems(), 0);
  renderSidebarItemList();
  renderSidebarStructure();
}

function moveMenuItem(index, direction) {
  const menu = currentMenu();
  if (!menu || isProtectedMenu(menu)) return;
  const nextIndex = direction === "up" ? index - 1 : index + 1;
  if (nextIndex < 0 || nextIndex >= menu.items.length) return;
  [menu.items[index], menu.items[nextIndex]] = [menu.items[nextIndex], menu.items[index]];
  state.selectedMenuItemIndex = nextIndex;
  renderMenuItemList();
  renderMenuStructure();
  refreshThemeMenusDirtyState();
}

function moveSidebarItem(index, direction) {
  const sidebar = currentSidebarSection();
  if (!sidebar || isProtectedSidebar(sidebar)) return;
  const nextIndex = direction === "up" ? index - 1 : index + 1;
  if (nextIndex < 0 || nextIndex >= sidebar.items.length) return;
  [sidebar.items[index], sidebar.items[nextIndex]] = [sidebar.items[nextIndex], sidebar.items[index]];
  state.selectedSidebarItemIndex = nextIndex;
  renderSidebarItemList();
  renderSidebarStructure();
  refreshCustomSidebarDirtyState();
}

function renderAll() {
  renderMenuSelector();
  renderMenuItemList();
  renderMenuStructure();
  renderSidebarSectionSelector();
  renderSidebarItemList();
  renderSidebarStructure();
}

/**
 * 生成后台菜单 API 的增量保存 payload。
 * @param {string} target - menus 表示只保存自定义菜单，sidebar 表示只保存自定义侧边栏。
 * @returns {{menus?: Array, sidebars?: Array}} 返回只包含当前编辑区域的请求体。
 */
function payload(target = "menus") {
  const body = {};
  if (target !== "sidebar") {
    body.menus = state.menus
      .filter((menu) => !isProtectedMenu(menu))
      .map((menu) => ({
        id: menu.id,
        name: String(menu.name || "").trim(),
        items: menu.items.map(serializeMenuItem)
      }));
  }
  if (target === "sidebar") {
    body.sidebars = state.sidebars
      .filter((sidebar) => !isProtectedSidebar(sidebar))
      .map((sidebar) => ({
        id: sidebar.id,
        name: String(sidebar.name || "").trim(),
        items: sidebar.items.map(serializeSidebarBlock)
      }));
  }
  return body;
}

/**
 * 判断自定义链接是否会通过后端保存归一化。
 *
 * 设计说明：
 * 后端会接受站内路径、http(s)、mailto，以及可补全为 https:// 的裸域名。
 * 前端保存前做同等校验，避免用户看到“保存成功”后项目被后端 normalize 阶段丢弃。
 *
 * @param {string} value - 用户输入的 URL。
 * @returns {boolean} 可保存时返回 true。
 */
function canPersistMenuURL(value) {
  const raw = String(value || "").trim();
  if (!raw) return false;
  const lower = raw.toLowerCase();
  if (raw.startsWith("/") && !raw.startsWith("//")) return true;
  if (lower.startsWith("https://") || lower.startsWith("http://") || lower.startsWith("mailto:")) return true;
  if (raw.startsWith("/") || /\s/.test(raw)) return false;
  try {
    const parsed = new URL(`https://${raw}`);
    const host = parsed.hostname || "";
    return Boolean(host && (host.toLowerCase() === "localhost" || host.includes(".")));
  } catch (_) {
    return false;
  }
}

/**
 * 校验一个菜单或侧边栏集合中是否有保存后会被丢弃的项目。
 *
 * @param {Array<{id: string, name: string, items: Array}>} groups - 菜单或侧边栏数组。
 * @param {string} groupLabel - 用于错误提示的集合名称，例如“菜单”。
 * @returns {string} 返回错误文案；空字符串表示可以保存。
 */
function validateMenuGroups(groups, groupLabel) {
  for (const group of groups) {
    if (isProtectedMenu(group)) continue;
    const groupName = String(group.name || group.id || "").trim() || groupLabel;
    const items = Array.isArray(group.items) ? group.items : [];
    for (let index = 0; index < items.length; index += 1) {
      const item = items[index];
      const title = itemTitle(item) || trFormat("menus.item.numbered", "Item {number}", { number: index + 1 });
      const prefix = trFormat("menus.validation.prefix", "{group} \"{groupName}\" item \"{title}\"", {
        group: groupLabel,
        groupName,
        title
      });
      const type = normalizeMenuItemType(item.type);
      if (type === "" && !String(item.label || "").trim() && !String(item.target || "").trim()) {
        return trFormat("menus.validation.no_content", "{prefix} has no content to save.", { prefix });
      }
      if (["page", "post", "tag"].includes(type) && !String(item.target || "").trim()) {
        return trFormat("menus.validation.missing_target", "{prefix} is missing a target.", { prefix });
      }
      if (type === "url" && !canPersistMenuURL(item.url)) {
        return trFormat("menus.validation.invalid_url", "{prefix} has a URL that cannot be saved.", { prefix });
      }
    }
  }
  return "";
}

/**
 * 校验侧边栏区块和自定义区域内部链接。
 * @returns {string} 返回第一条错误文案；空字符串表示可以保存。
 */
function validateSidebarGroups() {
  for (const sidebar of state.sidebars) {
    if (isProtectedSidebar(sidebar)) continue;
    const sidebarName = String(sidebar.name || sidebar.id || "").trim() || tr("sidebar.title", "Sidebar");
    const blocks = Array.isArray(sidebar.items) ? sidebar.items : [];
    for (const block of blocks) {
      const type = normalizeSidebarBlockType(block.type);
      const title = itemTitle(block) || sidebarBlockTypeLabel(type);
      if (type !== "custom") continue;
      const message = validateMenuGroups(
        [{ id: block.label, name: `${sidebarName} / ${title}`, items: block.items || [] }],
        tr("menus.validation.group.sidebar", "Sidebar")
      );
      if (message) return message;
    }
  }
  return "";
}

/**
 * 保存前按当前编辑区域校验 payload。
 *
 * @param {string} target - menus 表示保存自定义菜单，sidebar 表示保存自定义侧边栏。
 * @returns {string} 返回第一条错误文案；空字符串表示可以提交。
 */
function validateBeforeSave(target = "menus") {
  if (target === "sidebar") return validateSidebarGroups();
  return validateMenuGroups(state.menus, tr("menus.validation.group.menu", "Menu"));
}

if (menuSelector) {
  menuSelector.addEventListener("input", () => {
    const menu = currentMenu();
    if (!menu || isProtectedMenu(menu)) {
      renderMenuSelector();
      return;
    }
    menu.name = menuSelector.value;
    renderMenuSelector({ syncInput: false });
    refreshThemeMenusDirtyState();
  });
}

if (sidebarSectionSelector) {
  sidebarSectionSelector.addEventListener("input", () => {
    const sidebar = currentSidebarSection();
    if (!sidebar) return;
    if (isProtectedSidebar(sidebar)) {
      renderSidebarSectionSelector();
      return;
    }
    sidebar.name = sidebarSectionSelector.value;
    renderSidebarSectionSelector({ syncInput: false });
    refreshCustomSidebarDirtyState();
  });
}

if (menuSelectorToggle) {
  menuSelectorToggle.addEventListener("click", () => {
    if (!menuSelectorList) return;
    const open = menuSelectorList.hidden;
    if (sidebarSectionSelectorList) closeEditableSelector(sidebarSectionSelectorList, sidebarSectionSelectorToggle);
    menuSelectorList.hidden = !open;
    setEditableSelectorOpen(menuSelectorToggle, open);
  });
}

if (sidebarSectionSelectorToggle) {
  sidebarSectionSelectorToggle.addEventListener("click", () => {
    if (!sidebarSectionSelectorList) return;
    const open = sidebarSectionSelectorList.hidden;
    if (menuSelectorList) closeEditableSelector(menuSelectorList, menuSelectorToggle);
    sidebarSectionSelectorList.hidden = !open;
    setEditableSelectorOpen(sidebarSectionSelectorToggle, open);
  });
}

document.addEventListener("click", (event) => {
  const target = event.target;
  if (!(target instanceof Element)) return;
  if (!target.closest("#themeMenuSettings .editable-selector")) closeEditableSelector(menuSelectorList, menuSelectorToggle);
  if (!target.closest("#customSidebarSettings .editable-selector")) closeEditableSelector(sidebarSectionSelectorList, sidebarSectionSelectorToggle);
});

document.addEventListener("keydown", (event) => {
  if (event.key !== "Escape") return;
  closeEditableSelector(menuSelectorList, menuSelectorToggle);
  closeEditableSelector(sidebarSectionSelectorList, sidebarSectionSelectorToggle);
});

if (addThemeMenuButton) {
  addThemeMenuButton.addEventListener("click", () => {
    const name = tr("menus.menu.default_name", "New Menu");
    state.menus.push({ id: uniqueMenuID(name), name, items: [] });
    selectMenu(state.menus.length - 1);
    refreshThemeMenusDirtyState();
  });
}

if (addThemeMenuItemButton) {
  addThemeMenuItemButton.addEventListener("click", () => {
    const menu = currentMenu();
    if (!menu || isProtectedMenu(menu)) return;
    menu.items.push(createBlankMenuItem(menu.items));
    state.selectedMenuItemIndex = menu.items.length - 1;
    renderMenuItemList();
    renderMenuStructure();
    refreshThemeMenusDirtyState();
  });
}

if (addSidebarSectionButton) {
  addSidebarSectionButton.addEventListener("click", () => {
    const name = tr("sidebar.section.default_name", "New Sidebar");
    state.sidebars.push({ id: uniqueSidebarID(name), name, items: [] });
    selectSidebarSection(state.sidebars.length - 1);
    refreshCustomSidebarDirtyState();
  });
}

if (addSidebarSectionItemButton) {
  addSidebarSectionItemButton.addEventListener("click", () => {
    const sidebar = currentSidebarSection();
    if (!sidebar || isProtectedSidebar(sidebar)) return;
    sidebar.items.push(createBlankSidebarBlock(sidebar.items));
    state.selectedSidebarItemIndex = sidebar.items.length - 1;
    state.selectedSidebarCustomItemIndex = -1;
    renderSidebarItemList();
    renderSidebarStructure();
    refreshCustomSidebarDirtyState();
  });
}

if (deleteThemeMenuButton) {
  deleteThemeMenuButton.addEventListener("click", async () => {
    if (state.selectedIndex < 0) return;
    const menu = currentMenu();
    if (isProtectedMenu(menu)) return;
    if (menu && !window.confirm(tr("menus.confirm.delete", "Delete this menu?"))) return;
    state.menus.splice(state.selectedIndex, 1);
    selectMenu(state.selectedIndex);
    await saveMenus(tr("settings.status.deleting", "Deleting"));
  });
}

if (deleteSidebarSectionButton) {
  deleteSidebarSectionButton.addEventListener("click", async () => {
    if (state.selectedSidebarIndex < 0) return;
    const sidebar = currentSidebarSection();
    if (isProtectedSidebar(sidebar)) return;
    if (sidebar && !window.confirm(tr("sidebar.confirm.delete", "Delete this sidebar?"))) return;
    state.sidebars.splice(state.selectedSidebarIndex, 1);
    selectSidebarSection(state.selectedSidebarIndex);
    await saveMenus(tr("settings.status.deleting", "Deleting"), { target: "sidebar" });
  });
}

async function saveMenus(statusMessage, options = {}) {
  const target = options.target || "menus";
  if (target === "sidebar") {
    commitActiveCustomSidebarRename();
  } else {
    commitActiveThemeMenuRename();
  }
  const validationMessage = validateBeforeSave(target);
  if (validationMessage) {
    if (target === "sidebar") {
      setCustomSidebarStatus(validationMessage);
      setSaveCustomSidebarFeedback(tr("settings.status.error", "Error"), { resetAfter: 2000 });
    } else {
      setThemeMenuStatus(validationMessage);
      setSaveThemeMenusFeedback(tr("settings.status.error", "Error"), { resetAfter: 2000 });
    }
    return false;
  }
  const savingMessage = statusMessage || tr("settings.status.saving", "Saving");
  if (target === "sidebar") {
    setCustomSidebarStatus(savingMessage);
    setSaveCustomSidebarFeedback(savingMessage, { disabled: true });
  } else {
    setThemeMenuStatus(savingMessage);
    setSaveThemeMenusFeedback(savingMessage, { disabled: true });
  }
  let response;
  try {
    response = await fetch(apiURL("/admin/api/menus"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload(target))
    });
  } catch (error) {
    const message = error && error.message ? error.message : tr("settings.status.error", "Error");
    if (target === "sidebar") {
      setCustomSidebarStatus(message);
      setSaveCustomSidebarFeedback(tr("settings.status.error", "Error"), { resetAfter: 2000 });
    } else {
      setThemeMenuStatus(message);
      setSaveThemeMenusFeedback(tr("settings.status.error", "Error"), { resetAfter: 2000 });
    }
    return false;
  }
  if (!response.ok) {
    const message = (await response.text()).trim();
    if (target === "sidebar") {
      setCustomSidebarStatus(message);
      setSaveCustomSidebarFeedback(tr("settings.status.error", "Error"), { resetAfter: 2000 });
    } else {
      setThemeMenuStatus(message);
      setSaveThemeMenusFeedback(tr("settings.status.error", "Error"), { resetAfter: 2000 });
    }
    return false;
  }
  const savedSettings = await response.json().catch(() => null);
  if (savedSettings) {
    replaceState(normalizeState(savedSettings));
    resetThemeMenusSavedSnapshot();
    resetCustomSidebarSavedSnapshot();
    renderAll();
  }
  if (target === "sidebar") {
    setCustomSidebarStatus(tr("settings.status.saved", "Saved"));
    setSaveCustomSidebarFeedback(tr("settings.status.saved", "Saved"), { resetAfter: 1600 });
  } else {
    setThemeMenuStatus(tr("settings.status.saved", "Saved"));
    setSaveThemeMenusFeedback(saveThemeMenusDefaultLabel);
  }
  return true;
}

if (saveThemeMenusButton) {
  saveThemeMenusButton.addEventListener("click", async () => {
    if (!canSaveCurrentThemeMenu()) return;
    await saveMenus();
  });
}

if (saveCustomSidebarButton) {
  saveCustomSidebarButton.addEventListener("click", async () => {
    if (!canSaveCurrentCustomSidebar()) return;
    await saveMenus(undefined, { target: "sidebar" });
  });
}

resetThemeMenusSavedSnapshot();
resetCustomSidebarSavedSnapshot();
if (themeMenuRoot || customSidebarRoot) renderAll();
