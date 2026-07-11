require "minitest/autorun"
require "open3"

class LiveStreamingJsToolEventsTest < Minitest::Test
  VIEW_PATH = File.expand_path("../views/index.erb", __dir__)

  def test_subagent_prompt_prefers_call_arguments_and_falls_back_to_result_details
    script = File.read(VIEW_PATH)
    formatter_start = script.index("    function subagentPromptFromArguments")
    formatter_end = script.index("    function subagentResultFailed", formatter_start)
    formatter = script[formatter_start...formatter_end]
    assertion = <<~JS
      const fromArgs = subagentPromptFromEvent({ args: { task: "Original delegated prompt" }, partialResult: { details: { task: "Result prompt" } } });
      if (fromArgs !== "Original delegated prompt") process.exit(1);
      const fromDetails = subagentPromptFromEvent({ result: { details: { task: "Retained result prompt" } } });
      if (fromDetails !== "Retained result prompt") process.exit(2);
      if (subagentPromptFromEvent({ args: { task: {} }, result: { details: null } }) !== "") process.exit(3);
    JS

    refute_nil formatter_start
    refute_nil formatter_end
    _stdout, stderr, status = Open3.capture3("node", stdin_data: formatter + assertion)

    assert status.success?, stderr
  end

  def test_subagent_prompt_keeps_the_original_call_prompt_through_result_updates
    script = File.read(VIEW_PATH)
    renderer_start = script.index("    function renderSubagentPrompt")
    renderer_end = script.index("    const TOOL_OUTPUT_DESKTOP_TAIL_LINES", renderer_start)
    renderer = script[renderer_start...renderer_end]
    assertion = <<~JS
      global.document = {
        createElement(tagName) {
          return {
            tagName,
            className: "",
            dataset: {},
            children: [],
            textContent: "",
            append(...children) { this.children.push(...children); },
            setAttribute() {}
          };
        }
      };
      const entry = { details: { insertBefore(element) { this.prompt = element; } }, output: {} };
      renderSubagentPrompt(entry, "Original delegated prompt");
      renderSubagentPrompt(entry, "reviewer: Reconstructed result prompt");
      if (entry.subagentPromptPreview.textContent !== "Original delegated prompt") process.exit(1);
      if (entry.subagentPromptBody.textContent !== "Original delegated prompt") process.exit(2);
    JS

    refute_nil renderer_start
    refute_nil renderer_end
    _stdout, stderr, status = Open3.capture3("node", stdin_data: renderer + assertion)

    assert status.success?, stderr
  end

  def test_retained_general_subagent_progress_uses_final_fallback_only_after_completion
    script = File.read(VIEW_PATH)
    formatter_start = script.index("    function subagentResultFailed")
    formatter_end = script.index("    function toolExecutionContentText", formatter_start)
    formatter = script[formatter_start...formatter_end]
    assertion = <<~JS
      const details = { status: "running", tools: [], textItems: ["Full fresh answer"], streamingText: "", usage: {}, model: "provider/model" };
      if (subagentDisplayText(details, "Final answer", true).includes("Final answer")) process.exit(1);
      const fresh = subagentDisplayText(details, "Truncated final", false);
      if (!fresh.includes("Full fresh answer") || fresh.includes("Truncated final")) process.exit(2);
      const retained = subagentDisplayText(details, "Canonical final", false, true);
      if (!retained.includes("Canonical final") || retained.includes("Full fresh answer")) process.exit(3);
      const malformedRead = { status: "running", tools: [{ name: "read", status: "running", args: { path: "/tmp/file", offset: {}, limit: {} } }], textItems: [], usage: {} };
      if (subagentDisplayText(malformedRead, "", true).includes("NaN")) process.exit(4);
    JS

    _stdout, stderr, status = Open3.capture3("node", stdin_data: formatter + assertion)

    assert status.success?, stderr
  end

  def test_tool_events_clear_active_assistant_streaming_before_rendering_tools
    script = File.read(VIEW_PATH)
    tool_event = script.index('if (["tool_execution_start", "tool_execution_update", "tool_execution_end"].includes(event.type))')
    cleanup = script.index("clearLiveAssistantStreaming();", tool_event)
    render_tool = script.index("renderToolExecutionEvent(event);", tool_event)

    refute_nil cleanup
    refute_nil render_tool
    assert_operator cleanup, :<, render_tool
  end
end
