module Prompts
  class BashCommand
    attr_reader :command, :exclude_from_context

    def self.parse(message)
      message = message.to_s
      return unless message.start_with?("!")

      exclude_from_context = message.start_with?("!!")
      command = message.delete_prefix(exclude_from_context ? "!!" : "!").strip
      new(command, exclude_from_context: exclude_from_context) unless command.empty?
    end

    def initialize(command, exclude_from_context: false)
      @command = command
      @exclude_from_context = exclude_from_context
    end
  end
end
