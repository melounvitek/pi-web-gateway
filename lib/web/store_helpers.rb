require_relative "../gateway_read_state_store"
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
  end
end
