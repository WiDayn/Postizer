(() => {
  const panel = document.querySelector("[data-plugin-id]");
  if (!panel) return;

  const pluginID = panel.dataset.pluginId;
  const resultBox = document.querySelector("[data-plugin-action-result]");

  function tr(key, fallback) {
    if (window.postizerMessage) return window.postizerMessage(key, fallback);
    return fallback;
  }

  function apiURL(actionID) {
    return `/admin/api/plugins/${encodeURIComponent(pluginID)}/actions/${encodeURIComponent(actionID)}`;
  }

  function jobURL(jobID) {
    return `/admin/api/plugin-jobs/${encodeURIComponent(jobID)}`;
  }

  function setStatus(form, text, error = false) {
    const status = form.querySelector("[data-plugin-action-status]");
    if (!status) return;
    status.textContent = text;
    status.classList.toggle("is-error", error);
    status.classList.toggle("ui-status--error", error);
  }

  async function invoke(actionID, formData, form) {
    setStatus(form, "Working...");
    const response = await fetch(apiURL(actionID), {
      method: "POST",
      body: formData
    });
    const text = await response.text();
    if (!response.ok) {
      throw new Error(text || response.statusText);
    }
    return JSON.parse(text || "{}");
  }

  function applyFieldValues(scope, values) {
    Object.entries(values || {}).forEach(([name, value]) => {
      scope.querySelectorAll(`[data-plugin-action-form] [name="${CSS.escape(name)}"]`).forEach((field) => {
        if (!(field instanceof HTMLInputElement) || field.type === "file") return;
        if (field.type === "checkbox") {
          field.checked = ["1", "true", "yes", "on"].includes(String(value ?? "").toLowerCase());
          return;
        }
        field.value = String(value ?? "");
      });
    });
  }

  async function loadPageValues(page) {
    const actionID = page.dataset.pluginLoadAction;
    if (!actionID) return;
    try {
      const result = await invoke(actionID, new FormData(), page);
      applyFieldValues(page, result.field_values);
    } catch (error) {
      renderResult({ title: "加载设置失败", summary: error.message, level: "error" });
    }
  }

  function renderResult(result) {
    if (!resultBox) return;
    resultBox.innerHTML = "";
    resultBox.classList.toggle("is-empty", !result);
    if (!result) {
      resultBox.appendChild(renderEmptyResult());
      return;
    }

    applyFieldValues(panel, result.field_values);

    const fragment = document.createDocumentFragment();
    if (result.title || result.summary) {
      const header = document.createElement("div");
      header.className = "plugin-result-header";
      if (result.level) header.dataset.level = result.level;
      if (result.title) {
        const title = document.createElement("h3");
        title.textContent = result.title;
        header.appendChild(title);
      }
      if (result.summary) {
        const summary = document.createElement("p");
        summary.textContent = result.summary;
        header.appendChild(summary);
      }
      fragment.appendChild(header);
    }

    if (result.job) {
      fragment.appendChild(renderJob(result.job));
      pollJob(result.job.id);
    }

    (result.sections || []).forEach((section) => fragment.appendChild(renderSection(section)));

    if (result.next_actions && result.next_actions.length) {
      const actions = document.createElement("div");
      actions.className = "settings-actions";
      result.next_actions.forEach((action) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = `ui-button ${action.style === "primary" ? "ui-button--primary" : "ui-button--ghost"}`;
        button.textContent = action.label || action.id;
        button.addEventListener("click", async () => {
          if (action.confirm && !window.confirm(action.confirm)) return;
          const formData = new FormData();
          Object.entries(action.fields || {}).forEach(([key, value]) => formData.set(key, value));
          button.disabled = true;
          try {
            const nextResult = await invoke(action.id, formData, panel);
            renderResult(nextResult);
          } catch (error) {
            renderResult({ title: "Action failed", summary: error.message, level: "error" });
          } finally {
            button.disabled = false;
          }
        });
        actions.appendChild(button);
      });
      fragment.appendChild(actions);
    }

    resultBox.appendChild(fragment);
  }

  function renderEmptyResult() {
    const empty = document.createElement("div");
    empty.className = "plugin-result-empty";
    const title = document.createElement("h3");
    title.textContent = "No results yet";
    const text = document.createElement("p");
    text.textContent = "Action output will appear here.";
    empty.append(title, text);
    return empty;
  }

  function renderSection(section) {
    const block = document.createElement("section");
    block.className = "plugin-result-section";
    if (section.kind) block.dataset.kind = section.kind;
    if (section.title) {
      const title = document.createElement("h4");
      title.textContent = section.title;
      block.appendChild(title);
    }
    if (section.text) {
      const text = document.createElement("p");
      text.textContent = section.text;
      block.appendChild(text);
    }
    if (section.rows && section.rows.length) {
      const list = document.createElement("dl");
      list.className = "plugin-result-list";
      section.rows.forEach((row) => {
        const dt = document.createElement("dt");
        dt.textContent = row.label;
        const dd = document.createElement("dd");
        dd.appendChild(renderRowValue(row.value, section.kind));
        list.append(dt, dd);
      });
      block.appendChild(list);
    }
    if (section.links && section.links.length) {
      const links = document.createElement("nav");
      links.className = `plugin-result-links plugin-result-links--${section.kind || "default"}`;
      section.links.forEach((item) => {
        const link = document.createElement("a");
        link.className = `plugin-result-link${item.current ? " is-current" : ""}`;
        link.href = item.url || "#";
        link.textContent = item.count > 0 ? `${item.label} ${item.count}` : item.label;
        if (item.current) link.setAttribute("aria-current", "page");
        links.appendChild(link);
      });
      block.appendChild(links);
    }
    if (section.cards && section.cards.length) {
      const grid = document.createElement("div");
      grid.className = "plugin-card-grid plugin-card-grid--admin";
      section.cards.forEach((card) => grid.appendChild(renderCard(card)));
      block.appendChild(grid);
    }
    return block;
  }

  function renderCard(card) {
    const article = document.createElement("article");
    article.className = "plugin-cover-card";

    const poster = document.createElement("div");
    poster.className = "plugin-cover-card__poster";
    if (card.image_url) {
      const image = document.createElement("img");
      image.src = card.image_url;
      image.alt = card.title || "";
      image.loading = "lazy";
      image.addEventListener("error", () => {
        poster.classList.add("is-missing");
        image.remove();
      });
      poster.appendChild(image);
    } else {
      poster.classList.add("is-missing");
    }

    const body = document.createElement("div");
    body.className = "plugin-cover-card__body";
    const title = document.createElement("h3");
    title.textContent = card.title || "Untitled";
    body.appendChild(title);
    if (card.subtitle) {
      const subtitle = document.createElement("p");
      subtitle.textContent = card.subtitle;
      body.appendChild(subtitle);
    }
    if (card.added_at && !String(card.added_at).startsWith("0001-")) {
      const added = document.createElement("p");
      added.className = "plugin-cover-card__added";
      const label = document.createElement("span");
      label.textContent = tr("plugins.card.added_at", "Added");
      const timestamp = document.createElement("time");
      timestamp.dateTime = card.added_at;
      const parsed = new Date(card.added_at);
      timestamp.textContent = Number.isNaN(parsed.getTime()) ? card.added_at : parsed.toLocaleString();
      added.append(label, document.createTextNode(" "), timestamp);
      body.appendChild(added);
    }
    if (card.badges && card.badges.length) {
      const badges = document.createElement("div");
      badges.className = "plugin-cover-card__badges";
      card.badges.forEach((label) => {
        const badge = document.createElement("span");
        badge.textContent = label;
        badges.appendChild(badge);
      });
      body.appendChild(badges);
    }
    if (card.description) {
      const description = document.createElement("p");
      description.className = "plugin-cover-card__description";
      description.textContent = card.description;
      body.appendChild(description);
    }
    if (/^https?:\/\//i.test(card.url || "")) {
      const details = document.createElement("a");
      details.className = "plugin-cover-card__details";
      details.href = card.url;
      details.target = "_blank";
      details.rel = "noopener noreferrer";
      details.textContent = "查看资料";
      body.appendChild(details);
    }
    if (card.actions && card.actions.length) {
      const actions = document.createElement("div");
      actions.className = "plugin-cover-card__actions";
      card.actions.forEach((action) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = `ui-button ${action.style === "primary" ? "ui-button--primary" : "ui-button--ghost"}`;
        button.textContent = action.label || action.id;
        button.addEventListener("click", async () => {
          if (action.confirm && !window.confirm(action.confirm)) return;
          const formData = new FormData();
          Object.entries(action.fields || {}).forEach(([key, value]) => formData.set(key, value));
          button.disabled = true;
          try {
            renderResult(await invoke(action.id, formData, panel));
          } catch (error) {
            renderResult({ title: "Action failed", summary: error.message, level: "error" });
          } finally {
            button.disabled = false;
          }
        });
        actions.appendChild(button);
      });
      body.appendChild(actions);
    }

    article.append(poster, body);
    return article;
  }

  function renderRowValue(value, sectionKind) {
    const text = String(value || "");
    if (isDownloadURL(text)) {
      const link = document.createElement("a");
      link.href = text;
      link.textContent = sectionKind === "download" ? "Download archive" : text;
      if (sectionKind === "download") link.setAttribute("download", "");
      return link;
    }
    return document.createTextNode(text);
  }

  function isDownloadURL(value) {
    return /^\/admin\/api\/plugin-downloads\/[A-Za-z0-9_-]+$/.test(value);
  }

  function renderJob(job) {
    const shell = document.createElement("section");
    shell.className = "plugin-job";
    shell.dataset.jobId = job.id;

    const top = document.createElement("div");
    top.className = "plugin-job__top";
    const title = document.createElement("h4");
    title.textContent = `Import ${job.status || "running"}`;
    const meta = document.createElement("span");
    meta.textContent = `${job.done || 0}/${job.total || 0}`;
    top.append(title, meta);

    const progress = document.createElement("progress");
    progress.max = 100;
    progress.value = job.percent || 0;
    progress.textContent = `${job.percent || 0}%`;

    const log = document.createElement("ol");
    log.className = "plugin-job-log";
    (job.logs || []).forEach((line) => {
      const item = document.createElement("li");
      item.textContent = line;
      log.appendChild(item);
    });

    shell.append(top, progress, log);
    (job.sections || []).forEach((section) => shell.appendChild(renderSection(section)));
    return shell;
  }

  async function pollJob(jobID) {
    if (!jobID) return;
    const response = await fetch(jobURL(jobID));
    if (!response.ok) return;
    const job = await response.json();
    const existing = resultBox && resultBox.querySelector(`[data-job-id="${CSS.escape(jobID)}"]`);
    if (existing) {
      existing.replaceWith(renderJob(job));
    }
    if (job.status === "running") {
      window.setTimeout(() => pollJob(jobID), 1000);
    }
  }

  document.querySelectorAll("[data-plugin-action-form]").forEach((form) => {
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      if (form.dataset.requiresConfirmation === "true" && !window.confirm(tr("plugins.action.confirm", "Are you sure you want to run this action?"))) return;
      const actionID = form.dataset.actionId;
      const formData = new FormData(form);
      const submitter = form.querySelector("button[type='submit']");
      if (submitter) submitter.disabled = true;
      form.classList.add("is-working");
      try {
        const result = await invoke(actionID, formData, form);
        setStatus(form, "Done");
        renderResult(result);
      } catch (error) {
        setStatus(form, error.message, true);
        renderResult({ title: "Action failed", summary: error.message, level: "error" });
      } finally {
        form.classList.remove("is-working");
        if (submitter) submitter.disabled = false;
      }
    });
  });

  document.querySelectorAll("[data-plugin-load-action]").forEach((page) => loadPageValues(page));
})();
