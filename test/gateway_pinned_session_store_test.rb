require "minitest/autorun"
require "tmpdir"
require_relative "../lib/gateway_pinned_session_store"

class GatewayPinnedSessionStoreTest < Minitest::Test
  def test_persists_pinned_session_paths
    Dir.mktmpdir do |dir|
      path = File.join(dir, "pinned-sessions.json")
      session_path = "/sessions/important.jsonl"

      GatewayPinnedSessionStore.new(path: path).pin(session_path)

      assert_equal [session_path], GatewayPinnedSessionStore.new(path: path).pinned_paths
      assert_equal [session_path], JSON.parse(File.read(path))

      GatewayPinnedSessionStore.new(path: path).unpin(session_path)

      assert_empty GatewayPinnedSessionStore.new(path: path).pinned_paths
      assert_equal [], JSON.parse(File.read(path))
    end
  end

  def test_recovers_from_malformed_state
    Dir.mktmpdir do |dir|
      path = File.join(dir, "pinned-sessions.json")
      File.write(path, "not json")
      store = GatewayPinnedSessionStore.new(path: path)

      assert_empty store.pinned_paths

      store.pin("/sessions/important.jsonl")

      assert_equal ["/sessions/important.jsonl"], store.pinned_paths
    end
  end
end
