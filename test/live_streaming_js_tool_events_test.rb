require "minitest/autorun"
require "json"
require "open3"

class LiveStreamingJsToolEventsTest < Minitest::Test
  ASSETS = File.expand_path("../public/assets", __dir__)
  STYLESHEET_PATH = File.join(ASSETS, "app.css")

  def test_subagent_parser_prefers_call_arguments_and_retains_rich_progress
    result = run_javascript(<<~JS)
      const { LiveMessageParser } = await import(#{module_url("live_message_parser.js").to_json});
      const parser = new LiveMessageParser();
      const details = { status: "running", tools: [], textItems: ["Full fresh answer"], streamingText: "", usage: {}, model: "provider/model" };
      const retainedDetails = parser.retainedSubagentDetails(null, details);
      const progressDetails = {
        status: "done",
        tools: [{ name: "bash", args: { command: "printf done" }, status: "done", output: "done" }],
        textItems: ["### Findings\\n\\n**Safe**"],
        streamingText: "",
        usage: { turns: 2 },
        model: "provider/model"
      };
      const legacyDetails = {
        mode: "single",
        results: [{
          agent: "reviewer",
          agentSource: "user",
          exitCode: 0,
          stopReason: "stop",
          messages: [{ role: "assistant", content: [
            { type: "toolCall", name: "read", arguments: { path: "/tmp/file" } },
            { type: "text", text: "## Legacy result" }
          ] }],
          usage: { turns: 1 },
          model: "provider/model"
        }]
      };
      console.log(JSON.stringify({
        fromArgs: parser.subagentPromptFromEvent({ args: { task: "Original" }, partialResult: { details: { task: "Result" } } }),
        fromDetails: parser.subagentPromptFromEvent({ result: { details: { task: "Retained" } } }),
        restored: parser.subagentPromptFromEvent({ result: { details: { task: "Truncated" } } }, "Complete"),
        running: parser.subagentDisplayParts(details, "Final", true),
        fresh: parser.subagentDisplayParts(details, "Truncated final", false),
        retained: parser.subagentDisplayParts(retainedDetails, "Canonical final", false, true),
        separated: parser.subagentDisplayParts(progressDetails, "Truncated final", false),
        legacy: parser.subagentDisplayParts(legacyDetails, "Truncated final", false),
        unknown: parser.subagentDisplayParts({ custom: true }, "Plain **fallback**", false),
        retainedUnknown: parser.subagentDisplayParts(parser.retainedSubagentDetails(null, { custom: true }), "Plain **fallback**", false),
        malformedLegacy: [
          parser.subagentDisplayParts({ results: [null] }, "Malformed **fallback**", false),
          parser.subagentDisplayParts({ results: [{ messages: null }] }, "Malformed **fallback**", false),
          parser.subagentDisplayParts({ results: [{ messages: [{ role: "assistant", content: {} }] }] }, "Malformed **fallback**", false),
          parser.subagentDisplayParts({ results: [{ messages: [{ role: "assistant", content: [{ type: "text", text: {} }] }] }] }, "Malformed **fallback**", false)
        ]
      }));
    JS

    assert_equal "Original", result["fromArgs"]
    assert_equal "Retained", result["fromDetails"]
    assert_equal "Complete", result["restored"]
    refute_includes result.dig("running", "answer"), "Final"
    assert_includes result.dig("fresh", "answer"), "Full fresh answer"
    assert_includes result.dig("retained", "answer"), "Canonical final"
    assert_includes result.dig("separated", "progress"), "$ printf done"
    refute_includes result.dig("separated", "progress"), "Findings"
    assert_equal "### Findings\n\n**Safe**", result.dig("separated", "answer")
    assert_equal "2 turns provider/model", result.dig("separated", "usage")
    assert_includes result.dig("legacy", "progress"), "read /tmp/file"
    refute_includes result.dig("legacy", "progress"), "Legacy result"
    assert_equal "## Legacy result", result.dig("legacy", "answer")
    assert_equal "1 turn provider/model", result.dig("legacy", "usage")
    assert_equal({ "progress" => "Plain **fallback**", "answer" => "", "usage" => "" }, result["unknown"])
    assert_equal result["unknown"], result["retainedUnknown"]
    assert_equal Array.new(4, { "progress" => "Malformed **fallback**", "answer" => "", "usage" => "" }), result["malformedLegacy"]
  end

  def test_subagent_renderer_keeps_markdown_answer_outside_progress
    result = run_javascript(<<~JS)
      const { LiveMessageRenderer } = await import(#{module_url("live_message_renderer.js").to_json});
      const renderer = Object.create(LiveMessageRenderer.prototype);
      renderer.document = { createElement: (tagName) => ({ tagName, className: "", dataset: {}, hidden: false, textContent: "" }) };
      renderer.markdownRenderer = { render: (element, text) => { element.markdown = text; element.renderCount = (element.renderCount || 0) + 1; } };
      renderer.renderToolTranscriptBody = (body, text) => { body.progress = text; };
      const entry = { body: {}, details: { children: [], append(element) { this.children.push(element); } } };
      renderer.renderSubagentDisplay(entry, { progress: "$ command", answer: "### Result", usage: "2 turns" });
      renderer.renderSubagentDisplay(entry, { progress: "$ next", answer: "### Result", usage: "2 turns" });
      console.log(JSON.stringify({
        progress: entry.body.progress,
        answer: entry.subagentAnswerElement.markdown,
        answerClass: entry.subagentAnswerElement.className,
        renderCount: entry.subagentAnswerElement.renderCount,
        usage: entry.subagentUsageElement.textContent,
        order: entry.details.children.map((element) => element.dataset.subagentAnswer === "" ? "answer" : "usage")
      }));
    JS

    assert_equal "$ next", result["progress"]
    assert_equal "### Result", result["answer"]
    assert_includes result["answerClass"], "message-body--markdown"
    assert_equal 1, result["renderCount"]
    assert_equal "2 turns", result["usage"]
    assert_equal %w[answer usage], result["order"]
  end

  def test_subagent_prompt_renderer_keeps_original_prompt
    result = run_javascript(<<~JS)
      const { LiveMessageRenderer } = await import(#{module_url("live_message_renderer.js").to_json});
      const renderer = Object.create(LiveMessageRenderer.prototype);
      renderer.document = { createElement: (tagName) => element(tagName) };
      const entry = { details: { insertBefore(value) { this.prompt = value; } }, output: {} };
      renderer.renderSubagentPrompt(entry, "Original delegated prompt");
      renderer.renderSubagentPrompt(entry, "Reconstructed result prompt");
      console.log(JSON.stringify({ text: entry.subagentPromptPreview.textContent, children: entry.subagentPromptElement.children.length }));
      function element(tagName) { return { tagName, className: "", dataset: {}, children: [], textContent: "", append(...values) { this.children.push(...values); }, setAttribute() {} }; }
    JS

    assert_equal "Original delegated prompt", result["text"]
    assert_equal 1, result["children"]
  end

  def test_expanded_subagent_prompt_unclamps_the_same_preview
    stylesheet = File.read(STYLESHEET_PATH)
    assert_includes stylesheet, ".subagent-prompt[open] .subagent-prompt-preview { display: block; overflow: visible;"
    assert_includes stylesheet, "white-space: pre-wrap; -webkit-line-clamp: unset;"
  end

  def test_tool_event_orchestration_clears_assistant_streaming_first
    app = File.read(File.join(ASSETS, "app.js"))
    block = app[/if \(\["tool_execution_start".*?\n  \}/m]
    assert_operator block.index("liveMessageRenderer.clearLiveAssistantStreaming();"), :<, block.index("liveMessageRenderer.renderToolExecutionEvent(event);")
  end

  private

  def module_url(name) = "file://#{File.join(ASSETS, name)}"

  def run_javascript(source)
    stdout, stderr, status = Open3.capture3("node", "--input-type=module", "-e", source)
    assert status.success?, stderr
    JSON.parse(stdout)
  end
end
