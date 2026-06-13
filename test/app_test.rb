require "minitest/autorun"
require "rack/mock"
require "tmpdir"
require "json"
require "fileutils"
require_relative "../app"

class AppTest < Minitest::Test
  def test_posts_prompt_to_selected_session_and_redirects_back
    Dir.mktmpdir do |dir|
      path = write_session(dir)
      calls = []
      PiWebGateway.set :sessions_root, dir
      PiWebGateway.set :rpc_client_factory, [->(session_path) {
        calls << [:start, session_path]
        FakeRpcClient.new(calls)
      }]

      response = Rack::MockRequest.new(PiWebGateway).post(
        "/prompt",
        params: { "session" => path, "message" => "Hello Pi" }
      )

      assert_equal 303, response.status
      assert_equal [[ :start, path ], [ :prompt, "Hello Pi" ], [ :get_messages ], [ :close ]], calls
      assert_includes response["Location"], Rack::Utils.escape(path)
    end
  end

  private

  class FakeRpcClient
    def initialize(calls)
      @calls = calls
    end

    def prompt(message)
      @calls << [:prompt, message]
    end

    def get_messages
      @calls << [:get_messages]
    end

    def close
      @calls << [:close]
    end
  end

  def write_session(root)
    session_dir = File.join(root, "--project--")
    FileUtils.mkdir_p(session_dir)
    path = File.join(session_dir, "session.jsonl")
    File.write(path, JSON.generate({ type: "session", id: "session-1", cwd: "/tmp/project" }) + "\n")
    path
  end
end
