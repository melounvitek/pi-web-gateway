export function activateToolOutputRegion(body, { focus = false } = {}) {
  if (!body) return;
  body.tabIndex = 0;
  body.setAttribute("role", "region");
  body.setAttribute("aria-label", "Expanded tool output");
  if (focus) body.focus({ preventScroll: true });
}

export function deactivateToolOutputRegion(body) {
  if (!body) return;
  body.tabIndex = -1;
  body.removeAttribute("role");
  body.removeAttribute("aria-label");
}

export function enhanceMarkdownCodeBlocks(root, document = root?.ownerDocument || globalThis.document) {
  root?.querySelectorAll?.(".message-body--markdown pre:not([data-copy-enhanced])").forEach((pre) => {
    pre.dataset.copyEnhanced = "true";
    const wrapper = document.createElement("div");
    wrapper.className = "message-code-block";
    pre.before(wrapper);
    wrapper.append(pre);

    const button = document.createElement("button");
    button.type = "button";
    button.className = "copy-button code-block-copy-button";
    button.dataset.copyTarget = "code-block";
    button.textContent = "Copy";
    wrapper.append(button);
  });
}
