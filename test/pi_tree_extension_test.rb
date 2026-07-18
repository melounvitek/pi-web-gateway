# frozen_string_literal: true

require "minitest/autorun"
require "json"
require "open3"
require "tmpdir"

class PiTreeExtensionTest < Minitest::Test
  PI_EXECUTABLE = ENV.fetch("GRIPI_PI", IO.popen(["sh", "-c", "command -v pi"], &:read).strip)
  PI_PACKAGE_ROOT = File.expand_path("..", File.dirname(File.realpath(PI_EXECUTABLE)))
  JITI_PATH = File.join(PI_PACKAGE_ROOT, "node_modules/jiti/lib/jiti.mjs")
  PI_INDEX_PATH = File.join(PI_PACKAGE_ROOT, "dist/index.js")
  EXTENSION_PATH = File.expand_path("../pi_extensions/gripi-tree.ts", __dir__)

  def test_large_native_tree_is_compacted_before_crossing_the_extension_bridge
    Dir.mktmpdir do |dir|
      stdout, stderr, status = Open3.capture3(
        "node", "--input-type=module", "-", JITI_PATH, PI_INDEX_PATH, EXTENSION_PATH, dir,
        stdin_data: <<~"JS"
          const { createJiti } = await import(process.argv[2]);

          const jiti = createJiti(import.meta.url, {
            interopDefault: true,
            alias: { "@earendil-works/pi-coding-agent": process.argv[3] },
          });
          const loadExtension = await jiti.import(process.argv[4], { default: true });
          const commands = new Map();
          const events = new Map();
          loadExtension({
            registerCommand(name, command) { commands.set(name, command); },
            on(name, handler) { events.set(name, handler); },
            setLabel() {},
          });
          await events.get("session_start")({}, {
            cwd: process.argv[5],
            isProjectTrusted() { return true; },
          });

          const assistantCases = {
            2: { content: "Final answer", stopReason: "stop" },
            3: { content: "Working", stopReason: "toolUse" },
            4: { content: "Long answer", stopReason: "length" },
            5: { content: "Legacy answer" },
            6: { content: "Partial answer", stopReason: "aborted" },
            7: { content: "Failed answer", stopReason: "error" },
            8: { content: "  ", stopReason: "stop" },
            9: { content: "Commentary", stopReason: "stop", commentary: true },
          };
          const nodes = Array.from({ length: 1001 }, (_, index) => {
            const assistantCase = assistantCases[index];
            const role = index === 1 ? "toolResult" : assistantCase ? "assistant" : "user";
            const content = index === 0
              ? [{ type: "text", text: "x".repeat(20_000) }, { type: "image", data: "RAW_IMAGE_DATA", mimeType: "image/png" }]
              : [{
                  type: "text",
                  text: role === "toolResult" ? `Tool preview ${"y".repeat(20_000)}` : assistantCase?.content || `Prompt ${index}`,
                  ...(assistantCase?.commentary ? { textSignature: JSON.stringify({ v: 1, id: "message-1", phase: "commentary" }) } : {}),
                }];
            return {
              entry: {
                id: `entry-${index}`,
                parentId: null,
                type: "message",
                timestamp: `2026-06-13T10:${String(Math.floor(index / 60)).padStart(2, "0")}:${String(index % 60).padStart(2, "0")}Z${"t".repeat(2_000)}`,
                message: {
                  role,
                  content,
                  ...(assistantCase?.stopReason ? { stopReason: assistantCase.stopReason } : {}),
                },
              },
              children: [],
              label: index === 0 ? "checkpoint" : "l".repeat(2_000),
              labelTimestamp: "2026-06-13T10:00:00Z",

            };
          });
          let statusText;
          await commands.get("gripi_tree_snapshot").handler(
            `abc ${Buffer.from(JSON.stringify({ filter: "all" })).toString("base64url")}`,
            {
              sessionManager: {
                getTree() { return nodes; },
                getLeafId() { return "entry-8"; },
              },
              ui: { setStatus(_key, value) { statusText = value; } },
            },
          );

          const snapshot = JSON.parse(statusText);
          const first = snapshot.entries.find((entry) => entry.entryId === "entry-0");
          const tool = snapshot.entries.find((entry) => entry.entryId === "entry-1");
          const assistantEntries = snapshot.entries.filter((entry) => /^entry-[2-9]$/.test(entry.entryId));
          if (!snapshot.ok) throw new Error(snapshot.error);
          if (snapshot.entries.length >= 1000 || !snapshot.truncated || snapshot.totalEntries !== 1001) throw new Error("tree was not byte-capped");
          const current = snapshot.entries.filter((entry) => entry.current);
          const latest = snapshot.entries.filter((entry) => entry.latest);
          if (snapshot.leafId !== "entry-8" || current.length !== 1 || current[0].entryId !== "entry-8") throw new Error("leaf was not retained");
          if (latest.length !== 1 || latest[0].entryId !== "entry-1000") throw new Error("latest entry was not retained");
          if (first.label !== "checkpoint" || first.labelTimestamp !== "2026-06-13T10:00:00Z") throw new Error("label metadata missing");
          if (Buffer.byteLength(first.text, "utf8") > 512) throw new Error("preview was not capped");
          if (!tool.text.startsWith("Tool preview") || Buffer.byteLength(tool.text, "utf8") > 512) throw new Error("tool preview was not capped");
          const finalAssistantIds = assistantEntries.filter((entry) => entry.messageKind === "assistant-final").map((entry) => entry.entryId).sort();
          if (first.messageKind !== "user" || JSON.stringify(finalAssistantIds) !== JSON.stringify(["entry-2", "entry-4", "entry-5"])) throw new Error("conversation message kinds were not projected");
          if (statusText.includes("RAW_IMAGE_DATA") || statusText.includes("y".repeat(1_000))) throw new Error("large content crossed the bridge");
          const configuredFilter = snapshot.settings?.treeFilterMode;
          const skipPrompt = snapshot.settings?.branchSummary?.skipPrompt;
          if (snapshot.filter !== "all" || !["default", "no-tools", "user-only", "labeled-only", "all"].includes(configuredFilter) || typeof skipPrompt !== "boolean") throw new Error("effective settings missing");
          process.stdout.write(JSON.stringify({ bytes: Buffer.byteLength(statusText, "utf8") }));
        JS
      )

      assert status.success?, stderr
      assert_operator JSON.parse(stdout).fetch("bytes"), :<, 1_000_000
    end
  end
end
