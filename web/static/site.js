function readPostizerMessages() {
  const node = document.querySelector("#postizerMessages");
  if (!node) return {};
  try {
    return JSON.parse(node.textContent || "{}");
  } catch (_) {
    return {};
  }
}

const postizerMessages = readPostizerMessages();

function postizerMessage(key, fallback) {
  const value = postizerMessages[key];
  return typeof value === "string" && value.trim() ? value : fallback;
}

function postizerFormatMessage(key, fallback, replacements = {}) {
  let text = postizerMessage(key, fallback);
  Object.entries(replacements).forEach(([name, value]) => {
    text = text.replaceAll(`{${name}}`, String(value));
  });
  return text;
}

window.postizerMessage = postizerMessage;
window.postizerFormatMessage = postizerFormatMessage;

const loginNoticeDismissedKey = "postizer.loginNotice.dismissed";

/**
 * 判断后台导航组是否属于当前页面所在的大类。
 * @param {HTMLElement} group - 带有 data-admin-nav-group 的导航分组容器。
 * @returns {boolean} true 表示该分组包含当前高亮子项，需要保持展开。
 */
function adminNavGroupIsCurrent(group) {
  return Boolean(group && (group.classList.contains("is-current") || group.querySelector(".admin-nav-sub a.is-active")));
}

/**
 * 同步后台导航分组的视觉状态和辅助功能状态。
 * @param {HTMLElement} group - 需要展开或收起的导航分组容器。
 * @param {boolean} expanded - true 表示展开子菜单，false 表示收起子菜单。
 * @returns {void}
 */
function setAdminNavGroupExpanded(group, expanded) {
  if (!group) return;
  const toggle = group.querySelector(".admin-nav-disclosure");
  const submenu = group.querySelector(".admin-nav-sub");

  group.classList.toggle("is-expanded", expanded);
  if (submenu) submenu.hidden = !expanded;
  if (toggle) toggle.setAttribute("aria-expanded", String(expanded));
}

/**
 * 统一收敛后台导航组展开状态。
 * @param {HTMLElement[]} groups - 当前侧边栏内全部后台导航分组。
 * @param {HTMLElement | null} extraExpandedGroup - 用户手动额外展开的非当前分组。
 * @returns {void}
 */
function syncAdminNavGroupAccordion(groups, extraExpandedGroup = null) {
  groups.forEach((group) => {
    const isCurrent = adminNavGroupIsCurrent(group);

    // 当前页面所在的大类必须保持展开；额外展开项最多只能有一个。
    group.classList.toggle("is-current", isCurrent);
    setAdminNavGroupExpanded(group, isCurrent || group === extraExpandedGroup);
  });
}

/**
 * 绑定后台侧边栏中的可折叠菜单组。
 *
 * 设计思路：
 * - 当前页面所属大类始终展开，页面跳到哪个大类，哪个大类就在首屏展开。
 * - 除当前大类外，用户最多只能手动展开一个其他大类。
 * - 当用户继续展开另一个非当前大类时，之前手动展开的大类会自动收起。
 *
 * @returns {void}
 */
function setupAdminNavGroups() {
  const groups = Array.from(document.querySelectorAll("[data-admin-nav-group]"));
  let extraExpandedGroup = null;

  syncAdminNavGroupAccordion(groups);

  groups.forEach((group) => {
    const toggle = group.querySelector(".admin-nav-disclosure");
    if (!toggle) return;

    toggle.addEventListener("click", () => {
      if (adminNavGroupIsCurrent(group)) {
        syncAdminNavGroupAccordion(groups, extraExpandedGroup);
        return;
      }

      extraExpandedGroup = group.classList.contains("is-expanded") ? null : group;
      syncAdminNavGroupAccordion(groups, extraExpandedGroup);
    });
  });
}

function setupAdminLoginNotice() {
  const notice = document.querySelector("[data-login-notice]");
  if (!notice) return;
  const noticeKey = notice.dataset.loginNoticeKey || "";

  try {
    if (noticeKey && localStorage.getItem(loginNoticeDismissedKey) === noticeKey) {
      notice.hidden = true;
      return;
    }
  } catch (_) {
    // Storage can be unavailable; closing still works for the current page.
  }

  const closeButton = notice.querySelector("[data-login-notice-close]");
  if (!closeButton) return;
  closeButton.addEventListener("click", () => {
    notice.hidden = true;
    try {
      if (noticeKey) localStorage.setItem(loginNoticeDismissedKey, noticeKey);
    } catch (_) {
      // Ignore storage failures.
    }
  });
}

function bindFileDropZone(dropZone) {
  const input = dropZone.querySelector('input[type="file"]');
  if (!input) return;
  const fileName = dropZone.querySelector("[data-file-name]");
  const autoSubmit = dropZone.dataset.fileDropAutoSubmit === "true";

  const updateName = () => {
    if (!fileName) return;
    const file = input.files && input.files[0];
    fileName.textContent = file ? file.name : "";
  };

  const submitForm = () => {
    if (!autoSubmit || !input.files || !input.files.length || !input.form) return;
    if (typeof input.form.requestSubmit === "function") {
      input.form.requestSubmit();
    } else {
      input.form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    }
  };

  input.addEventListener("change", () => {
    updateName();
    submitForm();
  });

  ["dragenter", "dragover"].forEach((eventName) => {
    dropZone.addEventListener(eventName, (event) => {
      event.preventDefault();
      dropZone.classList.add("is-dragover");
    });
  });

  ["dragleave", "drop"].forEach((eventName) => {
    dropZone.addEventListener(eventName, () => {
      dropZone.classList.remove("is-dragover");
    });
  });

  dropZone.addEventListener("drop", (event) => {
    event.preventDefault();
    const files = event.dataTransfer && event.dataTransfer.files;
    if (!files || !files.length) return;
    try {
      input.files = files;
    } catch (_) {
      return;
    }
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

function bindFileDropZones(root = document) {
  root.querySelectorAll("[data-file-drop]").forEach(bindFileDropZone);
}

window.postizerBindFileDropZones = bindFileDropZones;

const mathDelimiters = [
  { left: "$$", right: "$$", display: true },
  { left: "\\[", right: "\\]", display: true },
  { left: "\\(", right: "\\)", display: false },
  { left: "$", right: "$", display: false }
];

function renderArticleMath(root) {
  if (!root || !window.renderMathInElement) return;
  renderMathInElement(root, {
    delimiters: mathDelimiters,
    ignoredTags: ["script", "noscript", "style", "textarea", "pre", "code"],
    throwOnError: false
  });
}

function numberArticleHeadings(root) {
  if (!root) return;
  const headings = Array.from(root.querySelectorAll("h1, h2, h3, h4, h5, h6"));
  if (!headings.length) return;

  const baseLevel = Math.min(...headings.map((heading) => Number(heading.tagName.slice(1))));
  const counts = [0, 0, 0, 0, 0, 0];

  headings.forEach((heading) => {
    const level = Number(heading.tagName.slice(1));
    const depth = Math.max(0, level - baseLevel);

    for (let i = 0; i < depth; i += 1) {
      if (counts[i] === 0) counts[i] = 1;
    }
    counts[depth] += 1;
    for (let i = depth + 1; i < counts.length; i += 1) {
      counts[i] = 0;
    }

    heading.dataset.headingNumber = counts.slice(0, depth + 1).join(".");
    heading.classList.add("has-heading-number");
  });
}

function enhanceArticleFigures(root) {
  if (!root) return;
  root.querySelectorAll(".article-figure img").forEach((image) => {
    image.tabIndex = 0;
    image.setAttribute("role", "button");
    if (!image.title) image.title = postizerMessage("site.image.open", "Open image");
  });
}

/**
 * 读取代码块声明的语言名。
 * @param {HTMLElement} pre - Goldmark/Chroma 输出的 pre 元素。
 * @returns {string} 返回适合展示的语言名；无法识别时返回本地化的通用“代码”文案。
 */
function articleCodeBlockLanguage(pre) {
  const code = pre && pre.querySelector("code");
  const raw = [
    pre && pre.dataset ? pre.dataset.language : "",
    code && code.dataset ? code.dataset.lang || code.dataset.language : "",
    code ? Array.from(code.classList).find((name) => name.startsWith("language-")) : ""
  ].find((value) => typeof value === "string" && value.trim());

  const normalized = String(raw || "").trim().replace(/^language-/, "").toLowerCase();
  if (!normalized || normalized === "fallback") {
    return postizerMessage("site.code.language_fallback", "Code");
  }

  const labels = {
    bash: "Bash",
    sh: "Shell",
    shell: "Shell",
    zsh: "Zsh",
    go: "Go",
    golang: "Go",
    js: "JavaScript",
    javascript: "JavaScript",
    ts: "TypeScript",
    typescript: "TypeScript",
    html: "HTML",
    css: "CSS",
    json: "JSON",
    md: "Markdown",
    markdown: "Markdown",
    py: "Python",
    python: "Python",
    yaml: "YAML",
    yml: "YAML",
    text: postizerMessage("site.code.language_fallback", "Code"),
    plaintext: postizerMessage("site.code.language_fallback", "Code")
  };

  return labels[normalized] || normalized.replace(/[-_]+/g, " ").replace(/\b\w/g, (char) => char.toUpperCase());
}

/**
 * 把文本写入剪贴板。
 * @param {string} text - 要复制的纯文本代码内容。
 * @returns {Promise<void>} 复制成功时 resolve；复制失败时 reject。
 */
async function copyTextToClipboard(text) {
  if (navigator.clipboard && window.isSecureContext) {
    await navigator.clipboard.writeText(text);
    return;
  }

  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.inset = "0 auto auto 0";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.select();

  try {
    const copied = document.execCommand("copy");
    if (!copied) throw new Error("copy command failed");
  } finally {
    textarea.remove();
  }
}

/**
 * 判断一个 Chroma 行内节点是否是行号。
 * @param {Element} node - Chroma 生成的每行第一个子元素候选。
 * @returns {boolean} true 表示该节点只承载行号，复制代码时应跳过。
 */
function isChromaInlineLineNumber(node) {
  if (!node || node.nodeType !== 1) return false;
  return /^\d+$/.test((node.textContent || "").trim());
}

/**
 * 从 Chroma 的单行包装节点中提取真实代码文本。
 * @param {Element} line - Chroma 输出的单行 span 包装元素。
 * @returns {string} 返回该行真实代码文本，不包含 inline line number（内联行号）。
 */
function chromaCodeLineText(line) {
  const children = Array.from(line.children || []);
  if (children.length >= 2 && isChromaInlineLineNumber(children[0])) {
    return children.slice(1).map((child) => child.textContent || "").join("");
  }
  return line.textContent || "";
}

/**
 * 提取代码块中适合复制到剪贴板的源码文本。
 * @param {HTMLElement} pre - 文章正文中的 pre 代码块。
 * @returns {string} 返回源码文本；当 Chroma 开启 inline line number 时会剥离行号。
 */
function articleCodeBlockCopyText(pre) {
  const source = pre && (pre.querySelector("code") || pre);
  if (!source) return "";

  const lines = Array.from(source.children || []).filter((child) => child.tagName === "SPAN");
  const onlyLineWrappers = Array.from(source.childNodes || []).every((child) => {
    if (child.nodeType === 3) return !(child.textContent || "").trim();
    return child.nodeType === 1 && child.tagName === "SPAN";
  });
  if (lines.length > 0 && onlyLineWrappers) {
    return lines.map(chromaCodeLineText).join("");
  }

  return source.textContent || source.innerText || "";
}

/**
 * 给单个代码块添加语言标签和复制按钮。
 * @param {HTMLElement} pre - 文章正文中的 pre 代码块。
 * @returns {void}
 */
function enhanceArticleCodeBlock(pre) {
  if (!pre || pre.closest(".code-block")) return;

  const language = articleCodeBlockLanguage(pre);
  const wrapper = document.createElement("div");
  wrapper.className = "code-block";

  const toolbar = document.createElement("div");
  toolbar.className = "code-block__toolbar";

  const languageLabel = document.createElement("span");
  languageLabel.className = "code-block__language";
  languageLabel.textContent = language;

  const copyButton = document.createElement("button");
  copyButton.type = "button";
  copyButton.className = "code-block__copy";
  const copyLabel = postizerFormatMessage("site.code.copy_aria", "Copy {language} code", { language });
  copyButton.setAttribute("aria-label", copyLabel);
  copyButton.title = copyLabel;

  // 保留原始 pre/code 结构，避免破坏 Chroma 已生成的高亮 span 和滚动行为。
  pre.before(wrapper);
  toolbar.append(languageLabel, copyButton);
  wrapper.append(toolbar, pre);

  copyButton.addEventListener("click", async () => {
    try {
      await copyTextToClipboard(articleCodeBlockCopyText(pre));
      copyButton.classList.add("is-complete");
      copyButton.classList.remove("is-error");
    } catch (_) {
      copyButton.classList.add("is-error");
      copyButton.classList.remove("is-complete");
    }

    window.setTimeout(() => {
      copyButton.classList.remove("is-complete", "is-error");
    }, 1600);
  });
}

/**
 * 增强文章正文中的所有代码块。
 * @param {HTMLElement} root - 文章正文或后台预览容器。
 * @returns {void}
 */
function enhanceArticleCodeBlocks(root) {
  if (!root) return;
  root.querySelectorAll("pre").forEach((pre) => {
    if (pre.querySelector("code")) enhanceArticleCodeBlock(pre);
  });
}

function enhanceArticle(root) {
  if (!root) return;
  numberArticleHeadings(root);
  enhanceArticleFigures(root);
  enhanceArticleCodeBlocks(root);
  renderArticleMath(root);
}

window.postizerEnhanceArticle = enhanceArticle;

let articleLightboxReady = false;

function setupArticleLightbox() {
  if (articleLightboxReady) return;
  articleLightboxReady = true;

  const lightbox = document.createElement("div");
  lightbox.id = "imageLightbox";
  lightbox.className = "image-lightbox";
  lightbox.setAttribute("role", "dialog");
  lightbox.setAttribute("aria-modal", "true");
  lightbox.setAttribute("aria-hidden", "true");

  const closeButton = document.createElement("button");
  closeButton.type = "button";
  closeButton.className = "image-lightbox__close";
  closeButton.textContent = postizerMessage("site.lightbox.close", "Close");

  const image = document.createElement("img");
  image.className = "image-lightbox__image";
  image.alt = "";

  const caption = document.createElement("p");
  caption.className = "image-lightbox__caption";

  lightbox.append(closeButton, image, caption);
  document.body.appendChild(lightbox);

  const close = () => {
    lightbox.classList.remove("is-open");
    lightbox.setAttribute("aria-hidden", "true");
    document.body.classList.remove("has-open-lightbox");
    image.removeAttribute("src");
    caption.textContent = "";
  };

  const open = (source) => {
    const figure = source.closest(".article-figure");
    const figureCaption = figure && figure.querySelector("figcaption");
    image.src = source.currentSrc || source.src;
    image.alt = source.alt || "";
    caption.textContent = figureCaption ? figureCaption.textContent.trim() : source.alt || "";
    lightbox.classList.add("is-open");
    lightbox.setAttribute("aria-hidden", "false");
    document.body.classList.add("has-open-lightbox");
    closeButton.focus();
  };

  document.addEventListener("click", (event) => {
    const source = event.target.closest(".article-body .article-figure img");
    if (source) {
      open(source);
      return;
    }
    if (event.target === lightbox || event.target === closeButton) {
      close();
    }
  });

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && lightbox.classList.contains("is-open")) {
      close();
      return;
    }
    const source = event.target.closest && event.target.closest(".article-body .article-figure img");
    if (source && (event.key === "Enter" || event.key === " ")) {
      event.preventDefault();
      open(source);
    }
  });
}

document.addEventListener("DOMContentLoaded", () => {
  setupAdminNavGroups();
  setupAdminLoginNotice();
  bindFileDropZones();
  setupArticleLightbox();
  document.querySelectorAll(".article-body").forEach(enhanceArticle);

  const input = document.querySelector("#searchInput");
  const results = document.querySelector("#searchResults");
  if (input && results) {
    fetch("/search-index.json")
      .then((response) => response.json())
      .then((docs) => {
        const render = () => {
          const q = input.value.trim().toLowerCase();
          results.innerHTML = "";
          docs
            .filter((doc) => !q || `${doc.title} ${doc.summary} ${(doc.tags || []).join(" ")}`.toLowerCase().includes(q))
            .slice(0, 50)
            .forEach((doc) => {
              const li = document.createElement("li");
              li.innerHTML = `<time>index</time><a class="post-list-title" href="${doc.url}"></a><span class="post-list-summary"></span><span class="post-list-tags"></span>`;
              li.querySelector("time").textContent = postizerMessage("site.search.index_label", "index");
              li.querySelector("a").textContent = doc.title;
              li.querySelector(".post-list-summary").textContent = doc.summary || "";
              li.querySelector(".post-list-tags").textContent = (doc.tags || []).map((tag) => `#${tag}`).join(" ");
              results.appendChild(li);
            });
        };
        input.addEventListener("input", render);
        render();
      });
  }
});
