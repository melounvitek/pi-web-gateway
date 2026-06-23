ENV["PI_GATEWAY_ADMIN_PASSWORD"] ||= "test-password"

require "minitest/autorun"
require "tmpdir"
require "json"
require_relative "../lib/sessions/session_view"
require_relative "../lib/sessions/sidebar"
require_relative "../lib/pi_session_store"
require_relative "../lib/gateway_read_state_store"
require_relative "../lib/pi_attachment_store"

class SessionsSessionViewTest < Minitest::Test
  def test_builds_existing_session_view_instance_variables
    Dir.mktmpdir do |dir|
      session_path = write_session(dir)
      read_state = GatewayReadStateStore.new(path: File.join(dir, "read-state.json"))

      view = Sessions::SessionView.build(
        sessions_root: dir,
        params: { "session" => session_path },
        include_conversation: true,
        read_state_store: read_state,
        attachment_store: PiAttachmentStore.new(root: File.join(dir, "attachments")),
        rpc_clients: inactive_rpc_clients,
        mark_selected_read: true,
        pending_session_cwd: ->(_path) {}
      )

      assignments = view.to_instance_variables
      assert_instance_of PiSessionStore, assignments.fetch(:@store)
      assert_equal [session_path], assignments.fetch(:@groups).values.flatten.map(&:path)
      assert_equal session_path, assignments.fetch(:@selected_session).path
      assert_instance_of Sessions::Sidebar, assignments.fetch(:@sidebar)
      assert_equal ["Hello"], assignments.fetch(:@messages).map(&:text)
      refute assignments.fetch(:@conversation_has_older_messages)
      assert_equal 0, assignments.fetch(:@conversation_older_message_count)
      assert_equal({}, assignments.fetch(:@attachment_counts))
      assert_instance_of PiSessionStore::Status, assignments.fetch(:@session_status)
      refute assignments.fetch(:@current_tree_leaf_known)
      refute assignments.fetch(:@viewing_older_tree_leaf)
    end
  end

  def test_conversation_uses_latest_window_for_long_history
    Dir.mktmpdir do |dir|
      session_path = write_session_with_messages(dir, 180)

      view = Sessions::SessionView.build(
        sessions_root: dir,
        params: { "session" => session_path },
        include_conversation: true,
        read_state_store: GatewayReadStateStore.new(path: File.join(dir, "read-state.json")),
        attachment_store: PiAttachmentStore.new(root: File.join(dir, "attachments")),
        rpc_clients: inactive_rpc_clients,
        mark_selected_read: false,
        pending_session_cwd: ->(_path) {}
      )

      assignments = view.to_instance_variables
      rendered_text = assignments.fetch(:@messages).map(&:text)
      assert_equal 150, rendered_text.length
      refute_includes rendered_text, "Message 30"
      assert_equal "Message 31", rendered_text.first
      assert_equal "Message 180", rendered_text.last
      assert assignments.fetch(:@conversation_has_older_messages)
      assert_equal 30, assignments.fetch(:@conversation_older_message_count)
    end
  end

  def test_conversation_renders_all_messages_when_within_window
    Dir.mktmpdir do |dir|
      session_path = write_session_with_messages(dir, 40)

      view = Sessions::SessionView.build(
        sessions_root: dir,
        params: { "session" => session_path },
        include_conversation: true,
        read_state_store: GatewayReadStateStore.new(path: File.join(dir, "read-state.json")),
        attachment_store: PiAttachmentStore.new(root: File.join(dir, "attachments")),
        rpc_clients: inactive_rpc_clients,
        mark_selected_read: false,
        pending_session_cwd: ->(_path) {}
      )

      assignments = view.to_instance_variables
      assert_equal (1..40).map { |index| "Message #{index}" }, assignments.fetch(:@messages).map(&:text)
      refute assignments.fetch(:@conversation_has_older_messages)
      assert_equal 0, assignments.fetch(:@conversation_older_message_count)
    end
  end

  def test_conversation_keeps_floor_when_messages_exceed_byte_budget
    Dir.mktmpdir do |dir|
      session_path = write_session_with_messages(dir, 100, text_suffix: "x" * 10_000)

      view = Sessions::SessionView.build(
        sessions_root: dir,
        params: { "session" => session_path },
        include_conversation: true,
        read_state_store: GatewayReadStateStore.new(path: File.join(dir, "read-state.json")),
        attachment_store: PiAttachmentStore.new(root: File.join(dir, "attachments")),
        rpc_clients: inactive_rpc_clients,
        mark_selected_read: false,
        pending_session_cwd: ->(_path) {}
      )

      assignments = view.to_instance_variables
      rendered_text = assignments.fetch(:@messages).map(&:text)
      assert_equal 50, rendered_text.length
      assert_match(/\AMessage 51/, rendered_text.first)
      assert_match(/\AMessage 100/, rendered_text.last)
      assert_equal 50, assignments.fetch(:@conversation_older_message_count)
    end
  end

  private

  def inactive_rpc_clients
    Class.new do
      def active?(_path)
        false
      end
    end.new
  end

  def write_session(root)
    path = File.join(root, "session.jsonl")
    File.write(path, [
      JSON.generate({ type: "session", id: "session", cwd: root }),
      JSON.generate({ type: "message", message: { role: "user", content: [{ type: "text", text: "Hello" }] } })
    ].join("\n") + "\n")
    path
  end

  def write_session_with_messages(root, count, text_suffix: "")
    path = File.join(root, "session.jsonl")
    entries = [{ type: "session", id: "session", cwd: root }] + (1..count).map do |index|
      { type: "message", message: { role: "user", content: [{ type: "text", text: "Message #{index}#{text_suffix}" }] } }
    end
    File.write(path, entries.map { |entry| JSON.generate(entry) }.join("\n") + "\n")
    path
  end
end
