require "minitest/autorun"
require "tmpdir"
require "json"
require "fileutils"
require_relative "../lib/browser_access_store"

class BrowserAccessStoreTest < Minitest::Test
  def setup
    @root = Dir.mktmpdir
    @path = File.join(@root, "browser-access.json")
  end

  def teardown
    FileUtils.remove_entry(@root) if @root && Dir.exist?(@root)
  end

  def test_prunes_stale_pending_requests_when_writing
    old_time = (Time.now.utc - (31 * 24 * 60 * 60)).iso8601
    recent_time = Time.now.utc.iso8601
    write_state(
      "approved_browsers" => [{ "token" => "approved", "approved_at" => old_time, "label" => "keep" }],
      "pending_requests" => [
        { "code" => "OLD1", "token" => "old-unrequested", "requested" => false, "created_at" => old_time, "requested_at" => nil },
        { "code" => "OLD2", "token" => "old-denied", "requested" => true, "created_at" => old_time, "requested_at" => old_time, "denied_at" => old_time },
        { "code" => "OLD3", "token" => "old-requested", "requested" => true, "created_at" => old_time, "requested_at" => old_time },
        { "code" => "NEW1", "token" => "recent", "requested" => true, "created_at" => recent_time, "requested_at" => recent_time }
      ]
    )

    BrowserAccessStore.new(path: @path).ensure_pending(token: "new", ip: "127.0.0.1", user_agent: "test")

    state = read_state
    assert_equal ["approved"], state.fetch("approved_browsers").map { |browser| browser.fetch("token") }
    assert_equal ["recent", "new"], state.fetch("pending_requests").map { |request| request.fetch("token") }
  end

  def test_prunes_stale_pending_requests_from_approved_check_at_most_once_per_day
    old_time = (Time.now.utc - (8 * 24 * 60 * 60)).iso8601
    write_state(
      "approved_browsers" => [{ "token" => "approved", "approved_at" => old_time, "label" => "keep" }],
      "pending_requests" => [
        { "code" => "OLD1", "token" => "old-unrequested", "requested" => false, "created_at" => old_time, "requested_at" => nil }
      ]
    )
    store = BrowserAccessStore.new(path: @path)

    assert store.approved?("approved")

    state = read_state
    assert_equal ["approved"], state.fetch("approved_browsers").map { |browser| browser.fetch("token") }
    assert_empty state.fetch("pending_requests")
  end

  def test_stale_pending_request_cannot_be_approved
    old_time = (Time.now.utc - (31 * 24 * 60 * 60)).iso8601
    write_state(
      "approved_browsers" => [],
      "pending_requests" => [
        { "code" => "OLD1", "token" => "old-requested", "requested" => true, "created_at" => old_time, "requested_at" => old_time }
      ]
    )

    result = BrowserAccessStore.new(path: @path).approve_code("OLD1")

    assert_nil result
    state = read_state
    assert_empty state.fetch("approved_browsers")
    assert_empty state.fetch("pending_requests")
  end

  def test_stale_pending_status_returns_created_after_pruning
    old_time = (Time.now.utc - (31 * 24 * 60 * 60)).iso8601
    write_state(
      "approved_browsers" => [],
      "pending_requests" => [
        { "code" => "OLD1", "token" => "old-requested", "requested" => true, "created_at" => old_time, "requested_at" => old_time }
      ]
    )

    status = BrowserAccessStore.new(path: @path).pending_status("old-requested")

    assert_equal "created", status
    assert_empty read_state.fetch("pending_requests")
  end

  private

  def write_state(state)
    File.write(@path, JSON.pretty_generate(state) + "\n")
  end

  def read_state
    JSON.parse(File.read(@path))
  end
end
