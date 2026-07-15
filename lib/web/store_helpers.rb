require_relative "../gateway_read_state_store"
require_relative "../gateway_pinned_session_store"
require_relative "../pi_attachment_store"

module Web
  module StoreHelpers
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

    def pinned_session_store
      if @pinned_session_store_path != settings.pinned_sessions_path
        @pinned_session_store_path = settings.pinned_sessions_path
        @pinned_session_store = GatewayPinnedSessionStore.new(path: settings.pinned_sessions_path)
      end
      @pinned_session_store
    end
  end
end
