require "json"
require "fileutils"
require "thread"

class GatewayPinnedSessionStore
  @mutexes = {}
  @mutexes_mutex = Mutex.new

  class << self
    def mutex_for(path)
      @mutexes_mutex.synchronize do
        @mutexes[path] ||= Mutex.new
      end
    end
  end

  def initialize(path:)
    @path = path
    @mutex = self.class.mutex_for(path)
  end

  def pinned_paths
    @mutex.synchronize { read_paths }
  end

  def pin(path)
    update_paths { |paths| paths << path unless paths.include?(path) }
  end

  def unpin(path)
    update_paths { |paths| paths.delete(path) }
  end

  private

  def update_paths
    @mutex.synchronize do
      paths = read_paths
      yield paths
      write_paths(paths)
    end
  end

  def read_paths
    return [] unless File.exist?(@path)

    parsed = JSON.parse(File.read(@path))
    parsed.is_a?(Array) ? parsed.select { |path| path.is_a?(String) }.uniq : []
  rescue JSON::ParserError, SystemCallError
    []
  end

  def write_paths(paths)
    FileUtils.mkdir_p(File.dirname(@path))
    temp_path = "#{@path}.tmp-#{$$}-#{Thread.current.object_id}"
    File.write(temp_path, JSON.pretty_generate(paths))
    File.rename(temp_path, @path)
  ensure
    File.delete(temp_path) if temp_path && File.exist?(temp_path)
  end
end
