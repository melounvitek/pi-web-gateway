module Rpc
  class PendingSessionRegistry
    Entry = Struct.new(:cwd, :created_at, :persisted_path, keyword_init: true)

    def initialize(entries = {}, clock: -> { Time.now })
      @clock = clock
      @entries = entries.to_h do |session_path, cwd|
        [session_path, Entry.new(cwd: cwd, created_at: @clock.call)]
      end
      @mutex = Mutex.new
    end

    def remember(session_path, cwd)
      @mutex.synchronize do
        entry = @entries[session_path]
        if entry
          entry.cwd = cwd
        else
          @entries[session_path] = Entry.new(cwd: cwd, created_at: @clock.call)
        end
      end
    end

    def cwd_for(session_path)
      @mutex.synchronize do
        @entries[session_path]&.cwd
      end
    end

    def remember_persisted_path(session_path, persisted_path)
      @mutex.synchronize do
        entry = @entries[session_path]
        entry.persisted_path = persisted_path if entry
      end
    end

    def persisted_path_for(session_path)
      @mutex.synchronize do
        @entries[session_path]&.persisted_path
      end
    end

    def paths
      @mutex.synchronize do
        @entries.filter_map { |session_path, entry| session_path unless entry.persisted_path }
      end
    end

    def entries
      @mutex.synchronize do
        @entries.filter_map { |session_path, entry| [session_path, entry.cwd] unless entry.persisted_path }
      end
    end

    def entries_with_created_at
      @mutex.synchronize do
        @entries.filter_map { |session_path, entry| [session_path, entry.cwd, entry.created_at] unless entry.persisted_path }
      end
    end

    def forget(session_path)
      @mutex.synchronize do
        @entries.delete(session_path)
      end
    end
  end
end
