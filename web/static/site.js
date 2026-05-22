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

function enhanceArticle(root) {
  if (!root) return;
  numberArticleHeadings(root);
  enhanceArticleFigures(root);
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
