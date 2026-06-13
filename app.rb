require "sinatra/base"
require "erb"
require_relative "lib/pi_session_store"
require_relative "lib/pi_rpc_client"

class PiWebGateway < Sinatra::Base
  set :root, File.dirname(__FILE__)
  set :sessions_root, ENV.fetch("PI_SESSIONS_ROOT", File.expand_path("~/.pi/agent/sessions"))
  set :rpc_client_factory, [->(session_path) { PiRpcClient.start(session_path) }]

  helpers do
    def h(value)
      ERB::Util.html_escape(value)
    end

    def selected?(session)
      @selected_session&.path == session.path
    end

    def format_time(time)
      time&.strftime("%Y-%m-%d %H:%M") || "unknown"
    end
  end

  get "/" do
    @store = PiSessionStore.new(root: settings.sessions_root)
    @groups = @store.grouped_sessions
    @selected_session = find_selected_session(@groups.values.flatten)
    @messages = @selected_session ? @store.messages(@selected_session.path) : []

    erb :index
  end

  post "/prompt" do
    session_path = params.fetch("session")
    message = params.fetch("message").to_s
    halt 400, "Message cannot be empty" if message.strip.empty?

    client = settings.rpc_client_factory.first.call(session_path)
    client.prompt(message)
    client.get_messages
    redirect "/?session=#{Rack::Utils.escape(session_path)}"
  ensure
    client&.close
  end

  private

  def find_selected_session(sessions)
    selected_path = params["session"]
    return sessions.first if selected_path.to_s.empty?

    sessions.find { |session| session.path == selected_path } || sessions.first
  end
end
