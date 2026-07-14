export class LiveMessageParser {
  constructor(homeDir = "") {
    const HOME_DIR = homeDir;

    function compactContentPart(part) {
      return part && typeof part === "object" && ["toolCall", "toolResult"].includes(part.type);
    }

    function subagentToolCall(part) {
      return part && typeof part === "object" && part.type === "toolCall" && part.name === "subagent";
    }

    function thinkingPart(part) {
      return part && typeof part === "object" && part.type === "thinking";
    }

    function textPartPhase(part) {
      if (!part || typeof part !== "object" || typeof part.textSignature !== "string" || !part.textSignature.startsWith("{")) return null;
      try {
        const signature = JSON.parse(part.textSignature);
        if (!signature || typeof signature !== "object" || signature.v !== 1 || typeof signature.id !== "string") return null;
        return ["commentary", "final_answer"].includes(signature.phase) ? signature.phase : null;
      } catch (_error) {
        return null;
      }
    }

    function finalAssistantTextPart(part) {
      if (typeof part === "string") return part.trim().length > 0;
      return part && typeof part === "object" && part.type === "text" && String(part.text || "").trim().length > 0 && textPartPhase(part) !== "commentary";
    }

    function stripThinkingHeading(text) {
      return String(text || "").replace(/^\s*\*\*[^\n*][^\n]*\*\*\s*\n{2,}/, "");
    }

    function bashCommandLine(part) {
      const args = part?.arguments || {};
      const timeout = args.timeout ? ` (timeout ${args.timeout}s)` : "";
      return `$ ${displayHomePath(args.command || "")}${timeout}`;
    }

    function displayHomePath(text) {
      if (!HOME_DIR) return String(text || "");
      return String(text || "").replace(new RegExp(`(^|[^A-Za-z0-9_.~/-])${escapeRegExp(HOME_DIR)}(?=/|$|[^A-Za-z0-9_.~/-])`, "g"), "$1~");
    }

    function escapeRegExp(text) {
      return String(text).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    }

    function contentPartText(part) {
      if (!part) return "";
      if (typeof part === "string") return part;
      if (part.text) return part.text;
      if (part.type === "thinking" || part.thinking) return stripThinkingHeading(part.thinking);
      if (part.type === "toolCall") {
        if (["bash", "read"].includes(part.name)) return "";
        if (["edit", "write"].includes(part.name)) return transcriptToolCallText(part.name, part.arguments || {});
        const args = part.arguments ? `\n${JSON.stringify(part.arguments, null, 2)}` : "";
        return `[tool: ${part.name || "tool"}]${args}`;
      }
      if (part.type === "toolResult") return part.output || part.result || "[tool result]";
      return "";
    }

    function transcriptToolCallText(name, args = {}) {
      if (name === "write") return previewText("+", args.content);
      if (name !== "edit") return "";

      const edits = Array.isArray(args.edits) ? args.edits : [];
      const editPreview = edits.map((edit, index) => {
        if (!edit || typeof edit !== "object") return "";
        return [
          `Edit ${index + 1}`,
          previewText("-", edit.oldText),
          previewText("+", edit.newText)
        ].filter(Boolean).join("\n");
      }).filter(Boolean).join("\n\n");

      return editPreview;
    }

    function previewText(prefix, text) {
      const lines = String(text || "").split("\n");
      if (lines[lines.length - 1] === "") lines.pop();
      if (lines.length === 0) return "";
      const preview = lines.slice(0, 6).map((line) => `${prefix} ${line}`);
      if (lines.length > 6) preview.push(`${prefix} …`);
      return preview.join("\n");
    }

    function toolSummaryParts(name, args = {}) {
      if (!["read", "edit", "write"].includes(name)) return null;
      return {
        name,
        path: displayHomePath(args.path || ""),
        range: name === "read" && args.offset && args.limit ? `${args.offset}-${Number(args.offset) + Number(args.limit) - 1}` : ""
      };
    }

    function contentPartLabel(part) {
      if (!part || typeof part !== "object") return null;
      if (part.type === "thinking" || part.thinking) return part.thinking ? "thinking" : null;
      if (part.type === "toolCall" && part.name === "bash") return bashCommandLine(part);
      if (part.type === "toolCall" || part.type === "toolResult") return part.name || part.toolName || "tool";
      return null;
    }

    function contentPartImage(part) {
      if (!part || typeof part !== "object" || part.type !== "image" || !part.data) return null;
      if (!["image/png", "image/jpeg", "image/gif", "image/webp"].includes(part.mimeType)) return null;
      return { src: `data:${part.mimeType};base64,${part.data}`, alt: "Attached image" };
    }

    function contentSegments(content, message = {}) {
      const parts = Array.isArray(content) ? content : [content];
      const groups = [];
      parts.forEach((part, index) => {
        if (subagentToolCall(part)) return;

        const compact = message.role === "toolResult" || compactContentPart(part);
        const bashCall = part && typeof part === "object" && part.type === "toolCall" && part.name === "bash";
        const image = part && typeof part === "object" && part.type === "image";
        const lastGroup = groups[groups.length - 1];
        if (image && lastGroup) {
          lastGroup.parts.push(part);
          lastGroup.endIndex = index;
        } else if (thinkingPart(part) && message.role !== "toolResult") {
          groups.push({ compact: false, parts: [part], startIndex: index, endIndex: index });
        } else if (!bashCall && !compact && lastGroup && lastGroup.compact === false && !thinkingPart(lastGroup.parts[0])) {
          lastGroup.parts.push(part);
          lastGroup.endIndex = index;
        } else {
          groups.push({ compact, parts: [part], startIndex: index, endIndex: index });
        }
      });

      return groups.map((group) => {
        const text = message.toolName === "edit" && message.details?.diff ? message.details.diff : group.parts.map(contentPartText).filter(Boolean).join("\n");
        const images = group.parts.map(contentPartImage).filter(Boolean);
        const labels = group.parts.map(contentPartLabel).filter(Boolean);
        const toolPart = group.parts.find((part) => part && typeof part === "object" && ["toolCall", "toolResult"].includes(part.type));
        const toolName = message.toolName || toolPart?.name || toolPart?.toolName;
        const summaryParts = toolSummaryParts(toolName, toolPart?.arguments || {});
        return {
          text,
          compact: group.compact,
          thinking: group.parts.length === 1 && thinkingPart(group.parts[0]) && message.role !== "toolResult",
          summary: message.toolName || [...new Set(labels)].join(" + ") || "tool output",
          summaryParts,
          error: message.isError === true,
          startIndex: group.startIndex,
          endIndex: group.endIndex,
          toolCallId: message.toolCallId || toolPart?.id || toolPart?.toolCallId,
          toolName,
          isToolResult: message.role === "toolResult" || toolPart?.type === "toolResult",
          toolTranscript: ["read", "edit", "write"].includes(toolName),
          toolPreview: toolPart?.type === "toolCall" && toolName === "edit",
          toolPrompt: toolName === "subagent" ? subagentPromptFromDetails(message.details) : "",
          finalAssistantResponse: !group.compact && group.parts.some(finalAssistantTextPart),
          images
        };
      }).filter((segment) => segment.text || segment.compact || segment.images.length > 0);
    }

    function eventMessage(event) {
      return event.message || event.delta || event.item || event;
    }

    function messageText(message) {
      if (!message) return "";
      if (typeof message === "string") return message;
      if (message.text) return message.text;
      if (message.content) return contentSegments(message.content, message).map((segment) => segment.text).join("\n");
      return "";
    }

    function liveEventRole(event, message) {
      if (["custom", "custom_message", "session_info", "queue_update", "compaction_start", "compaction_end"].includes(event.type)) {
        return "status";
      }

      const role = message?.role;
      if (["assistant", "user", "tool", "toolResult", "error"].includes(role)) return role;
      if (["custom", "system", "status"].includes(role)) return "status";
      if (["message_start", "message_update", "message_end"].includes(event.type)) return "assistant";
      return "status";
    }


    function toolExecutionResult(event) {
      return event.type === "tool_execution_update" ? event.partialResult : event.result;
    }

    function subagentPromptFromArguments(args) {
      if (!args || typeof args !== "object") return "";
      if (typeof args.task === "string" && args.task.trim()) return args.task;
      return subagentPromptList(args.tasks) || subagentPromptList(args.chain);
    }

    function subagentPromptList(items) {
      if (!Array.isArray(items)) return "";
      return items.filter((item) => item && typeof item.task === "string" && item.task.trim()).map((item) => item.agent ? `${item.agent}: ${item.task}` : item.task).join("\n\n");
    }

    function subagentPromptFromDetails(details) {
      if (!details || typeof details !== "object") return "";
      if (typeof details.task === "string" && details.task.trim()) return details.task;
      return subagentPromptList(details.results);
    }

    function subagentPromptFromEvent(event, restoredPrompt = "") {
      return restoredPrompt || subagentPromptFromArguments(event?.args) || subagentPromptFromDetails((event?.partialResult || event?.result)?.details);
    }

    function subagentDetailsFromEvent(event) {
      return toolExecutionResult(event)?.details;
    }

    function subagentRunning(event) {
      return event.type !== "tool_execution_end";
    }

    function subagentResultFailed(result) {
      return result.exitCode !== 0 || result.stopReason === "error" || result.stopReason === "aborted";
    }

    function subagentResultRunning(details, result, index, running) {
      if (result.exitCode === -1) return true;
      if (!running) return false;
      if (result.exitCode !== 0) return false;
      if (result.stopReason === "stop") return false;
      if (details.mode === "parallel") return true;
      return index === details.results.length - 1;
    }

    function subagentResultIcon(result, resultRunning = false) {
      if (resultRunning || result.exitCode === -1) return "⏳";
      return subagentResultFailed(result) ? "✗" : "✓";
    }

    function subagentFinalTextPart(messages = []) {
      const safeMessages = Array.isArray(messages) ? messages : [];
      for (let index = safeMessages.length - 1; index >= 0; index -= 1) {
        const message = safeMessages[index];
        if (message?.role !== "assistant") continue;
        const content = Array.isArray(message.content) ? message.content : [];
        const textPart = content.find((part) => part?.type === "text" && part.text);
        if (textPart) return textPart;
      }
      return null;
    }

    function subagentDisplayItems(messages = [], omittedPart = null) {
      const items = [];
      if (!Array.isArray(messages)) return items;
      messages.forEach((message) => {
        if (message?.role !== "assistant" || !Array.isArray(message.content)) return;
        message.content.forEach((part) => {
          if (part === omittedPart) return;
          if (part.type === "text" && part.text) items.push(part.text.split("\n").slice(0, 3).join("\n"));
          if (part.type === "toolCall") return items.push(`→ ${generalSubagentToolCall({ name: part.name, args: part.arguments || {} })}`);
        });
      });
      return items;
    }

    function subagentUsageText(usage = {}, model = "", costDigits = 3) {
      const value = (key) => {
        const number = Number(usage?.[key] ?? 0);
        return Number.isFinite(number) && number > 0 ? number : 0;
      };
      const parts = [];
      const turns = value("turns");
      if (turns) parts.push(`${turns} turn${turns === 1 ? "" : "s"}`);
      if (value("input")) parts.push(`↑${compactUsageNumber(value("input"))}`);
      if (value("output")) parts.push(`↓${compactUsageNumber(value("output"))}`);
      if (value("cacheRead")) parts.push(`R${compactUsageNumber(value("cacheRead"))}`);
      if (value("cacheWrite")) parts.push(`W${compactUsageNumber(value("cacheWrite"))}`);
      if (value("cost")) parts.push(`$${value("cost").toFixed(costDigits)}`);
      if (value("contextTokens")) parts.push(`ctx:${compactUsageNumber(value("contextTokens"))}`);
      if (model) parts.push(model);
      return parts.join(" ");
    }

    function compactUsageNumber(value) {
      const number = Number(value || 0);
      if (number < 1000) return String(Math.round(number));
      if (number < 1000000) return `${(number / 1000).toFixed(number < 10000 ? 1 : 0)}k`;
      return `${(number / 1000000).toFixed(1)}M`;
    }

    function generalSubagentDetails(details) {
      return details && Array.isArray(details.tools) && details.usage && typeof details.usage === "object" && !Array.isArray(details.usage);
    }

    function legacySubagentDetails(details) {
      return typeof details?.mode === "string" && Array.isArray(details.results) && details.results.length > 0 && details.results.every((result) => {
        if (!result || typeof result !== "object" || Array.isArray(result)) return false;
        if (typeof result.agent !== "string" || !Number.isInteger(result.exitCode)) return false;
        if (Object.hasOwn(result, "messages") && !Array.isArray(result.messages)) return false;
        return (result.messages || []).every((message) => {
          if (!message || typeof message !== "object" || Array.isArray(message)) return false;
          if (!Object.hasOwn(message, "content")) return true;
          if (!Array.isArray(message.content)) return false;
          return message.content.every((part) => part && typeof part === "object" && !Array.isArray(part) && (part.type !== "text" || !Object.hasOwn(part, "text") || typeof part.text === "string"));
        });
      });
    }

    function genericSubagentDetails(details) {
      if (!details || typeof details !== "object" || Array.isArray(details)) return true;
      return Object.keys(details).every((key) => ["agent", "cwd", "model", "task"].includes(key));
    }

    function generalSubagentStatusIcon(status) {
      if (status === "done") return "✓";
      if (status === "error") return "✗";
      return "⏳";
    }

    function generalSubagentIntegerArgument(value) {
      if (value === undefined || value === null) return null;
      const number = Number(value);
      return Number.isSafeInteger(number) ? number : null;
    }

    function generalSubagentToolCall(tool) {
      const args = tool?.args && typeof tool.args === "object" ? tool.args : {};
      const name = tool?.name || "tool";
      if (name === "bash") return `$ ${args.command || "..."}`;
      if (name === "read") {
        const path = args.path || args.file_path || "...";
        const offset = generalSubagentIntegerArgument(args.offset);
        const limit = generalSubagentIntegerArgument(args.limit);
        if (offset === null && limit === null) return `read ${path}`;
        const start = offset ?? 1;
        return `read ${path}:${start}${limit === null ? "" : `-${start + limit - 1}`}`;
      }
      if (["write", "edit"].includes(name)) return `${name} ${args.path || args.file_path || "..."}`;
      if (name === "grep") return `grep /${args.pattern || ""}/ in ${args.path || "."}`;
      if (name === "find") return `find ${args.pattern || "*"} in ${args.path || "."}`;
      if (name === "ls") return `ls ${args.path || "."}`;
      const serialized = JSON.stringify(args);
      const characters = [...serialized];
      return `${name} ${characters.length > 100 ? `${characters.slice(0, 100).join("")}…` : serialized}`;
    }

    function generalSubagentDisplayParts(details, fallback = "", preferFallback = false) {
      const lines = [`${generalSubagentStatusIcon(details.status)} general`];
      details.tools.forEach((tool) => {
        if (!tool || typeof tool !== "object" || Array.isArray(tool)) return;
        lines.push(`${generalSubagentStatusIcon(tool.status)} ${generalSubagentToolCall(tool)}`);
        const output = typeof tool.output === "string" ? tool.output.trim() : "";
        if (output) lines.push(...output.split("\n").map((line) => `  ${line}`));
      });
      const streamingText = typeof details.streamingText === "string" ? details.streamingText : "";
      const latestTextItem = Array.isArray(details.textItems) && typeof details.textItems.at(-1) === "string" ? details.textItems.at(-1) : "";
      const detailsText = streamingText || latestTextItem;
      return {
        progress: lines.join("\n"),
        answer: preferFallback ? fallback || detailsText : detailsText || fallback,
        usage: subagentUsageText(details.usage, details.model, 4)
      };
    }

    function subagentSummary(details, running = false) {
      if (generalSubagentDetails(details)) return "subagent general";
      if (!legacySubagentDetails(details)) return "subagent";
      if (details.mode === "single" && details.results.length === 1) return `subagent ${details.results[0].agent}`;
      const done = details.results.filter((result, index) => result.exitCode !== -1 && !subagentResultRunning(details, result, index, running)).length;
      const total = details.results.length;
      return `subagent ${details.mode} ${done}/${total}`;
    }

    function subagentDisplayParts(details, fallback = "", running = false, preferFallback = false) {
      if (generalSubagentDetails(details)) return generalSubagentDisplayParts(details, running ? "" : fallback, preferFallback);
      if (!legacySubagentDetails(details)) {
        if (genericSubagentDetails(details)) return { progress: running ? fallback : "", answer: running ? "" : fallback, usage: "" };
        return { progress: fallback, answer: "", usage: "" };
      }

      const lines = [];
      const answers = [];
      const usageItems = [];
      details.results.forEach((result, index) => {
        if (index > 0) lines.push("");
        const resultRunning = subagentResultRunning(details, result, index, running);
        lines.push(`${subagentResultIcon(result, resultRunning)} ${result.agent} (${result.agentSource || "unknown"})`);
        const finalPart = subagentFinalTextPart(result.messages);
        const items = subagentDisplayItems(result.messages, finalPart).slice(-10);
        if (items.length > 0) lines.push(...items);
        if (!finalPart && (result.errorMessage || result.stderr)) lines.push(result.errorMessage || result.stderr);
        if (finalPart?.text) answers.push(finalPart.text);
        const usage = subagentUsageText(result.usage, result.model, 4);
        if (usage) usageItems.push(usage);
      });
      const detailsAnswer = answers.join("\n\n");
      return {
        progress: lines.join("\n"),
        answer: preferFallback ? fallback || detailsAnswer : detailsAnswer || fallback,
        usage: usageItems.join(" | ")
      };
    }

    function richSubagentDetails(details) {
      return generalSubagentDetails(details) || legacySubagentDetails(details);
    }

    function retainedSubagentDetails(current, details, finalStatus = null) {
      let retained = richSubagentDetails(details) ? details : current || details;
      if (finalStatus && generalSubagentDetails(retained)) retained = { ...retained, status: finalStatus };
      return retained;
    }

    function toolExecutionContentText(event) {
      const content = event.partialResult?.content || event.result?.content;
      if (!content) return "";
      return contentSegments(content, { toolName: "tool" }).map((segment) => segment.text).join("\n");
    }

    function toolExecutionText(event) {
      if (event.type === "tool_execution_start") return "(running…)";
      return toolExecutionContentText(event) || (event.type === "tool_execution_end" ? "(done)" : "(running…)");
    }

    function toolExecutionSummary(event) {
      if (event.toolName === "subagent") return subagentSummary(subagentDetailsFromEvent(event), subagentRunning(event));
      const status = event.type === "tool_execution_end" ? (event.isError ? "failed" : "done") : "running";
      return `${event.toolName || "tool"} ${status}`;
    }


    function finalAssistantReplySegments(message) {
      if (!message?.content) {
        const text = messageText(message);
        return text ? [{ text }] : [];
      }

      return (Array.isArray(message.content) ? message.content : [message.content]).filter(finalAssistantTextPart).map((part) => ({
        text: typeof part === "string" ? part : part.text
      }));
    }

    this.contentSegments = contentSegments;
    this.eventMessage = eventMessage;
    this.messageText = messageText;
    this.liveEventRole = liveEventRole;
    this.finalAssistantReplySegments = finalAssistantReplySegments;
    this.finalAssistantReplyText = (message) => finalAssistantReplySegments(message).map((segment) => segment.text).join("\n");
    this.eventHasFinalAssistantText = (event) => finalAssistantReplySegments(eventMessage(event)).length > 0;
    this.displayHomePath = displayHomePath;
    this.subagentPromptFromDetails = subagentPromptFromDetails;
    this.subagentPromptFromEvent = subagentPromptFromEvent;
    this.subagentDetailsFromEvent = subagentDetailsFromEvent;
    this.subagentRunning = subagentRunning;
    this.subagentSummary = subagentSummary;
    this.subagentDisplayParts = subagentDisplayParts;
    this.richSubagentDetails = richSubagentDetails;
    this.retainedSubagentDetails = retainedSubagentDetails;
    this.toolExecutionContentText = toolExecutionContentText;
    this.toolExecutionText = toolExecutionText;
    this.toolExecutionSummary = toolExecutionSummary;
  }
}
