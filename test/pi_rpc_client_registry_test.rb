require "minitest/autorun"
require_relative "../lib/pi_rpc_client_registry"

class PiRpcClientRegistryTest < Minitest::Test
  def test_closes_clients_idle_longer_than_timeout
    now = Time.at(1_000)
    calls = []
    registry = PiRpcClientRegistry.new(factory: ->(_session_path) { FakeClient.new(calls) }, clock: -> { now })
    registry.ensure_client("/tmp/session.jsonl")

    now = Time.at(2_801)
    closed = registry.close_idle_clients(idle_timeout: 1_800)

    assert_equal ["/tmp/session.jsonl"], closed
    refute registry.active?("/tmp/session.jsonl")
    assert_equal [:close], calls
  end

  def test_keeps_recent_clients
    now = Time.at(1_000)
    calls = []
    registry = PiRpcClientRegistry.new(factory: ->(_session_path) { FakeClient.new(calls) }, clock: -> { now })
    registry.ensure_client("/tmp/session.jsonl")

    now = Time.at(2_799)
    closed = registry.close_idle_clients(idle_timeout: 1_800)

    assert_empty closed
    assert registry.active?("/tmp/session.jsonl")
    assert_empty calls
  end

  def test_keeps_excepted_clients
    now = Time.at(1_000)
    calls = []
    registry = PiRpcClientRegistry.new(factory: ->(_session_path) { FakeClient.new(calls) }, clock: -> { now })
    registry.ensure_client("/tmp/pending.jsonl")

    now = Time.at(2_801)
    closed = registry.close_idle_clients(idle_timeout: 1_800, except: ["/tmp/pending.jsonl"])

    assert_empty closed
    assert registry.active?("/tmp/pending.jsonl")
    assert_empty calls
  end

  def test_keeps_clients_currently_in_use
    now = Time.at(1_000)
    calls = []
    registry = PiRpcClientRegistry.new(factory: ->(_session_path) { FakeClient.new(calls) }, clock: -> { now })
    registry.ensure_client("/tmp/session.jsonl")

    registry.begin_use("/tmp/session.jsonl")
    now = Time.at(2_801)
    closed = registry.close_idle_clients(idle_timeout: 1_800)

    assert_empty closed
    assert registry.active?("/tmp/session.jsonl")
    assert_empty calls
  ensure
    registry.end_use("/tmp/session.jsonl") if registry
  end

  class FakeClient
    def initialize(calls)
      @calls = calls
    end

    def close
      @calls << :close
    end
  end
end
