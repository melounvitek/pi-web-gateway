const tabs = document.getElementById("tabs");
const content = document.getElementById("content");

let config = null;
let setupDraft = null;
const webviews = new Map();
const offlineGateways = new Map();

window.piGatewayDesktop.onAddGatewayRequested(() => {
  setupDraft = { name: "", url: "http://localhost:4567/" };
  render();
});

loadConfig();

async function loadConfig() {
  config = await window.piGatewayDesktop.getGatewayConfig();
  render();
}

function render() {
  if (!config) return;

  renderTabs();
  renderContent();
}

function renderTabs() {
  const showTabs = config.gateways.length > 1 || setupDraft;
  document.body.classList.toggle("has-tabs", Boolean(showTabs));
  tabs.hidden = !showTabs;
  tabs.replaceChildren();

  if (!showTabs) return;

  for (const gateway of config.gateways) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `tab${gateway.id === config.activeGatewayId && !setupDraft ? " active" : ""}`;
    button.textContent = gateway.name;
    button.addEventListener("click", async () => {
      setupDraft = null;
      config = await window.piGatewayDesktop.activateGateway(gateway.id);
      render();
    });
    tabs.append(button);
  }

  if (setupDraft) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "tab active";
    button.textContent = "New Gateway";
    tabs.append(button);
  }
}

function renderContent() {
  ensureWebviews();
  hideAllWebviews();
  removePanels();

  if (setupDraft) {
    content.append(setupPanel(setupDraft));
    return;
  }

  const gateway = activeGateway();
  const offlineReason = offlineGateways.get(gateway.id);
  if (offlineReason) {
    content.append(offlinePanel(gateway, offlineReason));
    return;
  }

  webviews.get(gateway.id).hidden = false;
}

function ensureWebviews() {
  const gatewayIds = new Set(config.gateways.map((gateway) => gateway.id));

  for (const [gatewayId, webview] of webviews) {
    if (!gatewayIds.has(gatewayId)) {
      webview.remove();
      webviews.delete(gatewayId);
    }
  }

  for (const gateway of config.gateways) {
    const existingWebview = webviews.get(gateway.id);
    if (existingWebview) {
      if (existingWebview.src !== gateway.url) existingWebview.src = gateway.url;
      continue;
    }

    const webview = document.createElement("webview");
    webview.partition = `persist:pi-gateway-${gateway.id}`;
    webview.src = gateway.url;
    webview.addEventListener("did-finish-load", () => {
      offlineGateways.delete(gateway.id);
      render();
    });
    webview.addEventListener("did-fail-load", (event) => {
      if (event.isMainFrame === false || event.errorCode === -3) return;
      offlineGateways.set(gateway.id, event.errorDescription || `Could not load ${gateway.url}`);
      render();
    });
    webviews.set(gateway.id, webview);
    content.append(webview);
  }
}

function hideAllWebviews() {
  for (const webview of webviews.values()) webview.hidden = true;
}

function removePanels() {
  for (const panel of content.querySelectorAll(".panel")) panel.remove();
}

function activeGateway() {
  return config.gateways.find((gateway) => gateway.id === config.activeGatewayId) || config.gateways[0];
}

function setupPanel(draft) {
  return gatewayFormPanel({
    title: "Add Gateway",
    description: "Add a trusted Pi Web Gateway URL to open it in its own isolated tab.",
    gateway: draft,
    saveLabel: "Add and Open",
    cancelLabel: config.gateways.length > 0 ? "Cancel" : null,
    onSave: async ({ name, url }) => {
      config = await window.piGatewayDesktop.addGateway({ name, url });
      setupDraft = null;
      render();
    },
    onCancel: () => {
      setupDraft = null;
      render();
    }
  });
}

function offlinePanel(gateway, reason) {
  return gatewayFormPanel({
    title: `${gateway.name} is not reachable`,
    description: "Start the gateway in a terminal, retry the saved URL, or update this tab to another trusted gateway URL.",
    gateway,
    saveLabel: "Save and Retry",
    cancelLabel: "Retry",
    details: `${gateway.url} — ${reason}`,
    onSave: async ({ name, url }) => {
      config = await window.piGatewayDesktop.saveGateway({ id: gateway.id, name, url });
      offlineGateways.delete(gateway.id);
      render();
    },
    onCancel: () => {
      offlineGateways.delete(gateway.id);
      reloadGateway(gateway.id);
      render();
    }
  });
}

function gatewayFormPanel({ title, description, gateway, saveLabel, cancelLabel, details, onSave, onCancel }) {
  const panel = document.createElement("section");
  panel.className = "panel";

  const card = document.createElement("form");
  card.className = "card";
  card.innerHTML = `
    <h1></h1>
    <p class="description"></p>
    <label>
      Name
      <input name="name" type="text" autocomplete="off" required>
    </label>
    <label>
      Gateway URL
      <input name="url" type="url" spellcheck="false" autocomplete="url" required>
    </label>
    <div class="actions"></div>
    <p class="muted"></p>
    <p class="error" role="alert"></p>
  `;

  card.querySelector("h1").textContent = title;
  card.querySelector(".description").textContent = description;
  card.elements.name.value = gateway.name || "";
  card.elements.url.value = gateway.url || "http://localhost:4567/";
  card.querySelector(".muted").textContent = details || "";

  const actions = card.querySelector(".actions");
  const saveButton = document.createElement("button");
  saveButton.type = "submit";
  saveButton.className = "primary";
  saveButton.textContent = saveLabel;
  actions.append(saveButton);

  if (cancelLabel) {
    const cancelButton = document.createElement("button");
    cancelButton.type = "button";
    cancelButton.className = "secondary";
    cancelButton.textContent = cancelLabel;
    cancelButton.addEventListener("click", onCancel);
    actions.append(cancelButton);
  }

  card.addEventListener("submit", async (event) => {
    event.preventDefault();
    card.querySelector(".error").textContent = "";

    try {
      await onSave({
        name: card.elements.name.value,
        url: card.elements.url.value
      });
    } catch (error) {
      card.querySelector(".error").textContent = error.message || "Could not save this gateway.";
    }
  });

  panel.append(card);
  return panel;
}

function reloadGateway(gatewayId) {
  const webview = webviews.get(gatewayId);
  if (webview) webview.reload();
}
