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
    if (!image.title) image.title = "Open image";
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
  lightbox.innerHTML = '<button type="button" class="image-lightbox__close">Close</button><img class="image-lightbox__image" alt=""><p class="image-lightbox__caption"></p>';
  document.body.appendChild(lightbox);

  const closeButton = lightbox.querySelector(".image-lightbox__close");
  const image = lightbox.querySelector(".image-lightbox__image");
  const caption = lightbox.querySelector(".image-lightbox__caption");

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
