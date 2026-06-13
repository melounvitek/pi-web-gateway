require "json"
require "open3"

class PiRpcClient
  def self.start(session_path, popen: Open3.method(:popen3))
    stdin, stdout, stderr, wait_thread = popen.call("pi", "--mode", "rpc", "--session", session_path)
    new(stdin: stdin, stdout: stdout, stderr: stderr, wait_thread: wait_thread)
  end

  def initialize(stdin:, stdout:, stderr: nil, wait_thread: nil)
    @stdin = stdin
    @stdout = stdout
    @stderr = stderr
    @wait_thread = wait_thread
    @request_sequence = 0
  end

  def get_state
    request("get_state", id: next_id("get_state"))
  end

  def get_messages
    request("get_messages", id: next_id("get_messages"))
  end

  def prompt(message)
    request("prompt", id: next_id("prompt"), message: message)
  end

  def abort
    request("abort", id: next_id("abort"))
  end

  def close
    close_io(@stdin)
    close_io(@stdout)
    close_io(@stderr)
    terminate_process
  end

  def request(type, id:, **payload)
    command = payload.merge(id: id, type: type)
    @stdin.write(JSON.generate(command) + "\n")
    @stdin.flush if @stdin.respond_to?(:flush)

    each_response do |response|
      return response if response["id"] == id
    end

    nil
  end

  private

  def close_io(io)
    io&.close unless io&.closed?
  rescue IOError
    nil
  end

  def terminate_process
    return unless @wait_thread&.respond_to?(:pid)

    Process.kill("TERM", @wait_thread.pid)
  rescue Errno::ESRCH, IOError
    nil
  end

  def next_id(type)
    @request_sequence += 1
    "#{type}-#{@request_sequence}"
  end

  def each_response
    while (line = @stdout.gets)
      next if line.strip.empty?

      begin
        yield JSON.parse(line)
      rescue JSON::ParserError
        next
      end
    end
  end
end
