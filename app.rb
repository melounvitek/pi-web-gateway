require "sinatra/base"
require "erb"
require "json"
require_relative "lib/rendering/markdown_renderer"
require_relative "lib/rpc/pending_session_registry"
require_relative "lib/sessions/session_family"
require_relative "lib/sessions/sidebar"
require "ipaddr"
require_relative "lib/pi_session_store"
require_relative "lib/pi_attachment_store"
require_relative "lib/gateway_read_state_store"
require_relative "lib/web/browser_access"
require_relative "lib/web/pwa_routes"
require_relative "lib/web/session_view_routes"
require_relative "lib/web/session_action_routes"
require_relative "lib/pi_rpc_client"
require_relative "lib/pi_rpc_client_registry"
require_relative "lib/time_formatter"

class PiWebGateway < Sinatra::Base
  register Web::BrowserAccess
  register Web::PwaRoutes
  register Web::SessionViewRoutes
  register Web::SessionActionRoutes

  set :root, File.dirname(__FILE__)
  set :sessions_root, ENV.fetch("PI_SESSIONS_ROOT", File.expand_path("~/.pi/agent/sessions"))
  set :attachments_root, ENV.fetch("PI_ATTACHMENTS_ROOT", File.expand_path("~/.pi/web-gateway/attachments"))
  GATEWAY_ENV_PATH = ENV.fetch("PI_GATEWAY_ENV_PATH", File.expand_path("~/.config/pi-web-gateway/env"))

  def self.load_gateway_env_file
    return unless File.exist?(GATEWAY_ENV_PATH)

    File.readlines(GATEWAY_ENV_PATH).each do |line|
      next if line.strip.empty? || line.strip.start_with?("#")

      key, value = line.split("=", 2)
      next unless key && value
      next if ENV.key?(key)

      ENV[key] = value.strip.sub(/\A(['"])(.*)\1\z/, "\\2")
    end
  end

  def self.permitted_hosts_from_env
    ENV.fetch("PI_GATEWAY_PERMITTED_HOSTS", "")
      .split(",")
      .map(&:strip)
      .reject(&:empty?)
  end

  def self.development_permitted_hosts
    [
      "localhost",
      ".localhost",
      ".test",
      IPAddr.new("0.0.0.0/0"),
      IPAddr.new("::/0")
    ]
  end

  load_gateway_env_file
  set :host_authorization, lambda {
    configured_hosts = permitted_hosts_from_env
    if development?
      { permitted_hosts: development_permitted_hosts + configured_hosts }
    elsif configured_hosts.empty?
      {}
    else
      { permitted_hosts: configured_hosts }
    end
  }

  gateway_admin_password = ENV["PI_GATEWAY_ADMIN_PASSWORD"].to_s
  if gateway_admin_password.empty?
    raise "PI_GATEWAY_ADMIN_PASSWORD is required. Set it in #{GATEWAY_ENV_PATH} or in the gateway process environment."
  end

  set :read_state_path, ENV.fetch("PI_READ_STATE_PATH", File.expand_path("~/.pi/web-gateway/read-state.json"))
  set :browser_access_path, ENV.fetch("PI_BROWSER_ACCESS_PATH", File.expand_path("~/.pi/web-gateway/browser-access.json"))
  set :gateway_admin_password, gateway_admin_password
  set :rpc_client_factory, [->(session_path) { PiRpcClient.start(session_path) }]
  set :new_rpc_client_factory, [->(cwd) { PiRpcClient.start_in_cwd(cwd) }]
  set :rpc_client_registry, nil
  set :pending_session_registry, Rpc::PendingSessionRegistry.new
  set :rpc_idle_timeout_seconds, ENV.fetch("PI_RPC_IDLE_TIMEOUT_SECONDS", "1800").to_i

  helpers do
    def h(value)
      ERB::Util.html_escape(value)
    end

    RECENT_SIDEBAR_SESSION_LIMIT = Sessions::Sidebar::RECENT_SESSION_LIMIT
    SIDEBAR_SESSION_PAGE_SIZE = Sessions::Sidebar::SESSION_PAGE_SIZE

    def selected?(session)
      @sidebar.selected?(session)
    end

    def unread?(session)
      @sidebar.unread?(session)
    end

    def session_classes(session, *classes)
      (classes + [selected?(session) ? "selected" : nil, unread?(session) ? "unread" : nil]).compact.join(" ")
    end

    def sorted_sidebar_sessions
      @sidebar.sorted_sessions
    end

    def sidebar_current_session
      @sidebar.current_session
    end

    def unread_sidebar_sessions
      @sidebar.unread_sessions
    end

    def unread_sidebar_session_count
      @sidebar.unread_session_count
    end

    def unread_sidebar_session_count_label
      @sidebar.unread_session_count_label
    end

    def unread_sidebar_session_aria_label
      @sidebar.unread_session_aria_label
    end

    def regular_sidebar_sessions
      @sidebar.regular_sessions
    end

    def regular_sidebar_session_pool
      @sidebar.regular_session_pool
    end

    def recent_sidebar_sessions
      @sidebar.recent_sessions
    end

    def show_all_sidebar_sessions?
      @sidebar.show_all_sessions?
    end

    def sidebar_sessions_limit
      @sidebar.sessions_limit
    end

    def sidebar_sessions_limit_param
      @sidebar.sessions_limit_param
    end

    def sidebar_sessions_overflow?
      @sidebar.sessions_overflow?
    end

    def sidebar_next_sessions_limit
      @sidebar.next_sessions_limit
    end

    def sidebar_sessions_remaining_count
      @sidebar.sessions_remaining_count
    end

    def sidebar_sessions_load_more_url
      @sidebar.sessions_load_more_url
    end

    def known_session_cwds
      @sidebar.known_session_cwds
    end

    def new_session_cwds
      preferred_cwd = selected_project_cwd || @selected_session&.cwd
      return known_session_cwds unless preferred_cwd

      [preferred_cwd, *known_session_cwds.reject { |cwd| cwd == preferred_cwd }]
    end

    def selected_project_cwd
      return @sidebar.selected_project_cwd if defined?(@sidebar) && @sidebar

      project = params["project"].to_s
      project.empty? ? nil : project
    end

    def project_label(session)
      File.basename(session.cwd.to_s)
    end

    def sidebar_session_search_query
      return @sidebar.search_query if defined?(@sidebar) && @sidebar

      params["session_search"].to_s.strip
    end

    def sidebar_session_search?
      return @sidebar.search? if defined?(@sidebar) && @sidebar

      !sidebar_session_search_query.empty?
    end

    def session_matches_sidebar_search?(session)
      @sidebar.matches_search?(session)
    end

    def sidebar_search_clear_url
      @sidebar.search_clear_url
    end

    def session_url(session_path)
      @sidebar.session_url(session_path)
    end

    def session_parent(session)
      @session_family.parent(session)
    end

    def session_children(session)
      @session_family.children(session)
    end

    def session_child_count(session)
      @session_family.child_count(session)
    end

    def session_family_root(session)
      @session_family.root(session)
    end

    def format_time(time)
      time&.localtime&.strftime("%Y-%m-%d %H:%M") || "unknown"
    end

    def format_relative_time(time)
      TimeFormatter.relative(time)
    end

    def message_role_key(role)
      case role.to_s
      when "assistant"
        "assistant"
      when "user"
        "user"
      when "tool", "toolResult"
        "tool"
      when "error"
        "error"
      when "system", "status"
        "status"
      else
        "status"
      end
    end

    def message_role_label(role)
      case role.to_s
      when "assistant"
        "pi"
      when "toolResult"
        "tool result"
      when "status"
        "status"
      else
        role.to_s.empty? ? "status" : role.to_s
      end
    end

    def message_article_class(message)
      classes = ["message", "message--#{message_role_key(message.role)}"]
      classes << "message--compact" if message.compact
      classes << "message--thinking" if message.thinking
      classes << "message--tool-transcript" if message.tool_transcript
      classes << "message--tool-error" if message.error
      classes.join(" ")
    end

    def message_metadata(message)
      format_time(message.timestamp) if message.timestamp
    end

    def message_fingerprint(role, text, timestamp)
      timestamp_key = message_timestamp_key(timestamp)
      return unless timestamp_key

      "#{message_role_key(role)}:#{timestamp_key}:#{stable_text_hash(normalized_message_text(text))}"
    end

    def message_timestamp_key(timestamp)
      timestamp&.to_i&.to_s
    end

    def normalized_message_text(text)
      text.to_s.gsub(/\r\n?/, "\n").strip
    end

    def stable_text_hash(text)
      text.bytes.reduce(5381) { |hash, byte| ((hash << 5) + hash + byte) & 0xffffffff }.to_s(16)
    end

    def attachment_label(count)
      "📎 #{count} image attachment#{count == 1 ? "" : "s"}"
    end

    def render_message_body(message)
      return h(message.text) if message.thinking
      return h(message.text) unless message.role == "assistant" && !message.compact

      markdown_renderer.render(message.text)
    end

    def render_compact_message_body(message)
      return h(message.text) unless message.tool_transcript && %w[edit write].include?(message.tool_name)

      message.text.to_s.lines(chomp: true).map do |line|
        %(<span class="tool-diff-line #{h(tool_diff_line_class(line))}">#{h(line)}</span>)
      end.join
    end

    def tool_diff_line_class(line)
      case line
      when /\A\+/
        "tool-diff-line--add"
      when /\A-/
        "tool-diff-line--remove"
      when /\A(?:Edit \d+|write\b|Wrote\b)/
        "tool-diff-line--meta"
      else
        "tool-diff-line--context"
      end
    end

    def session_status_items(status)
      return [] unless status

      [
        ["CTX", format_context_usage(status)],
        ["Model", format_model_with_thinking(status)]
      ].select { |_label, value| value.to_s != "" }
    end

    def format_context_usage(status)
      return if status.context_tokens.nil?

      usage = if status.context_limit
        percent = status.context_percent || ((status.context_tokens.to_f / status.context_limit) * 100).round(1)
        "#{percent}%/#{compact_number(status.context_limit)}"
      else
        compact_number(status.context_tokens)
      end
      status.context_estimated ? "≈#{usage}" : usage
    end

    def format_model(status)
      [status.provider, status.model_id].compact.reject(&:empty?).join("/")
    end

    def format_model_with_thinking(status)
      model = format_model(status)
      return if model.empty?

      [model, status.thinking_level.to_s.empty? ? nil : "(#{status.thinking_level})"].compact.join(" ")
    end

    def compact_number(value)
      number = value.to_f
      return value.to_s if number <= 0
      return number.round.to_s if number < 1_000
      return "#{(number / 1_000).round(1)}k" if number < 1_000_000

      "#{(number / 1_000_000).round(1)}M"
    end

    def markdown_renderer
      @markdown_renderer ||= Redcarpet::Markdown.new(
        Rendering::MarkdownRenderer.new(
          filter_html: true,
          hard_wrap: true,
          link_attributes: { rel: "nofollow noopener noreferrer", target: "_blank" }
        ),
        autolink: true,
        fenced_code_blocks: true,
        no_intra_emphasis: true,
        space_after_headers: true,
        strikethrough: true,
        tables: true
      )
    end
  end

  before do
    enforce_browser_access
    cleanup_idle_rpc_clients
  end


  private

  def attachment_store
    PiAttachmentStore.new(root: settings.attachments_root)
  end

  def read_state_store
    if @read_state_store_path != settings.read_state_path
      @read_state_store_path = settings.read_state_path
      @read_state_store = GatewayReadStateStore.new(path: settings.read_state_path)
    end
    @read_state_store
  end


  def rpc_clients
    settings.rpc_client_registry ||= PiRpcClientRegistry.new(factory: settings.rpc_client_factory.first)
  end

  def cleanup_idle_rpc_clients
    timeout = settings.rpc_idle_timeout_seconds
    return unless timeout.positive?

    rpc_clients.close_idle_clients(
      idle_timeout: timeout,
      except: pending_rpc_cwd_paths
    )
  end

  def with_rpc_client(session_path)
    session_path = canonical_rpc_session_path(session_path)
    rpc_clients.with_client(session_path) { |client| yield client }
  end

  def canonical_rpc_session_path(session_path)
    remapped_path = remap_active_pending_rpc_client(session_path)
    return remapped_path if remapped_path

    remap_pending_rpc_client(session_path) unless rpc_clients.active?(session_path)
    session_path
  end

  def remap_active_pending_rpc_client(session_path)
    cwd = pending_rpc_cwd(session_path)
    return unless cwd && rpc_clients.active?(session_path)

    real_path = session_file_from(rpc_clients.client_for(session_path).get_state)
    return unless real_path && File.exist?(real_path) && session_cwd(real_path) == cwd

    rpc_clients.move(session_path, real_path)
    forget_pending_rpc_cwd(session_path)
    real_path
  end

  def remap_pending_rpc_client(session_path)
    return unless File.exist?(session_path)

    pending_path = matching_pending_rpc_path(session_path)
    return unless pending_path

    rpc_clients.move(pending_path, session_path)
    forget_pending_rpc_cwd(pending_path)
  end

  def matching_pending_rpc_path(session_path)
    pending_rpc_cwd_entries.find do |pending_path, cwd|
      next unless rpc_clients.active?(pending_path)
      next unless session_cwd(session_path) == cwd

      session_file_from(rpc_clients.client_for(pending_path).get_state) == session_path
    end&.first
  end

  def pending_rpc_cwd(session_path)
    pending_session_registry.cwd_for(session_path)
  end

  def pending_rpc_cwd_paths
    pending_session_registry.paths
  end

  def pending_rpc_cwd_entries
    pending_session_registry.entries
  end

  def forget_pending_rpc_cwd(session_path)
    pending_session_registry.forget(session_path)
  end

  def pending_session_registry
    settings.pending_session_registry
  end

  def session_cwd(session_path)
    PiSessionStore.new(root: settings.sessions_root).sessions.find { |session| session.path == session_path }&.cwd
  end

  def response_data(response)
    response.is_a?(Hash) && response["data"].is_a?(Hash) ? response["data"] : response
  end

  def session_file_from(response)
    data = response_data(response)
    return unless data.is_a?(Hash)

    data["sessionFile"] || data["session_file"] || data["path"]
  end

end
