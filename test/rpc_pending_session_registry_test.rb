require "minitest/autorun"
require_relative "../lib/rpc/pending_session_registry"

class RpcPendingSessionRegistryTest < Minitest::Test
  def test_tracks_pending_session_cwds_and_creation_times
    now = Time.at(1_000)
    registry = Rpc::PendingSessionRegistry.new(clock: -> { now })

    registry.remember("/tmp/pending-1.jsonl", "/tmp/project-1")
    now = Time.at(1_100)
    registry.remember("/tmp/pending-2.jsonl", "/tmp/project-2")

    assert_equal "/tmp/project-1", registry.cwd_for("/tmp/pending-1.jsonl")
    assert_equal ["/tmp/pending-1.jsonl", "/tmp/pending-2.jsonl"], registry.paths
    assert_equal [["/tmp/pending-1.jsonl", "/tmp/project-1"], ["/tmp/pending-2.jsonl", "/tmp/project-2"]], registry.entries
    assert_equal [
      ["/tmp/pending-1.jsonl", "/tmp/project-1", Time.at(1_000)],
      ["/tmp/pending-2.jsonl", "/tmp/project-2", Time.at(1_100)]
    ], registry.entries_with_created_at

    registry.forget("/tmp/pending-1.jsonl")

    assert_nil registry.cwd_for("/tmp/pending-1.jsonl")
    assert_equal ["/tmp/pending-2.jsonl"], registry.paths
  end

  def test_preserves_a_persisted_alias_without_listing_it_as_pending
    registry = Rpc::PendingSessionRegistry.new({ "/tmp/pending.jsonl" => "/tmp/project" })

    registry.remember_persisted_path("/tmp/pending.jsonl", "/tmp/persisted.jsonl")

    assert_equal "/tmp/project", registry.cwd_for("/tmp/pending.jsonl")
    assert_equal "/tmp/persisted.jsonl", registry.persisted_path_for("/tmp/pending.jsonl")
    assert_empty registry.paths
    assert_empty registry.entries
    assert_empty registry.entries_with_created_at
  end

  def test_remembering_the_same_path_preserves_its_creation_time
    now = Time.at(1_000)
    registry = Rpc::PendingSessionRegistry.new(clock: -> { now })
    registry.remember("/tmp/pending.jsonl", "/tmp/project")

    now = Time.at(2_000)
    registry.remember("/tmp/pending.jsonl", "/tmp/renamed-project")

    assert_equal [["/tmp/pending.jsonl", "/tmp/renamed-project", Time.at(1_000)]], registry.entries_with_created_at
  end
end
