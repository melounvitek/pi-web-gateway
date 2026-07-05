require "minitest/autorun"

class LiveStreamingJsToolEventsTest < Minitest::Test
  VIEW_PATH = File.expand_path("../views/index.erb", __dir__)

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
