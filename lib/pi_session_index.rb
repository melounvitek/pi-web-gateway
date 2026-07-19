require "json"
require "digest"
require_relative "pi_session_index_scanner"

class PiSessionIndex
  MATERIALIZED_ENTRY_BYTES = 256 * 1024
  SCAN_CHUNK_BYTES = 64 * 1024

  # A non-nil byte bound must never exceed the corresponding fully rendered message size.
  Segment = Struct.new(:role, :tool_call_id, :tool_name, :minimum_window_bytes, :paired_minimum_window_bytes, keyword_init: true)
  Entry = Struct.new(
    :ordinal,
    :offset,
    :length,
    :type,
    :id,
    :parent_id,
    :target_id,
    :role,
    :segments,
    :subagent_tool_call_ids,
    :status_data,
    :estimate_text_length,
    :deferred_metadata,
    keyword_init: true
  )

  attr_reader :path, :entries, :by_id, :device, :inode, :size, :mtime_ns, :fingerprint, :complete, :supported, :ends_with_newline

  def self.build(path, previous: nil, &metadata_from_entry)
    3.times do
      index = new(path, previous: previous, &metadata_from_entry)
      return index if index.valid_for_current_path?
    rescue Errno::EAGAIN
      previous = nil
    end
    raise Errno::EAGAIN, "Session file kept changing while it was indexed: #{path}"
  end

  def initialize(path, previous: nil, &metadata_from_entry)
    @path = path
    @metadata_from_entry = metadata_from_entry
    @entries = previous ? previous.entries.dup : []
    @by_id = previous ? previous.by_id.dup : {}
    @complete = previous ? previous.complete : true
    @supported = previous ? previous.supported : true
    @ends_with_newline = previous ? previous.ends_with_newline : true

    File.open(path, "rb") do |file|
      before = file.stat
      digest = Digest::SHA256.new
      start_offset = extension_offset(previous, before, file, digest)
      file.seek(start_offset)
      scan(file, start_offset, digest)
      after = file.stat
      raise Errno::EAGAIN unless same_stat?(before, after)

      @device = after.dev
      @inode = after.ino
      @size = after.size
      @mtime_ns = stat_mtime_ns(after)
      @fingerprint = digest.hexdigest
    end
  end

  def valid_for_current_path?
    stat = File.stat(path)
    stat.dev == device && stat.ino == inode && stat.size == size && stat_mtime_ns(stat) == mtime_ns
  rescue SystemCallError
    false
  end

  def estimated_bytes
    256 + entries.sum do |entry|
      segment_bytes = Array(entry.segments).sum do |segment|
        64 + [segment.role, segment.tool_call_id, segment.tool_name].compact.sum(&:bytesize)
      end
      176 + [entry.type, entry.id, entry.parent_id, entry.target_id, entry.role].compact.sum(&:bytesize) +
        segment_bytes + Array(entry.subagent_tool_call_ids).sum { |id| 40 + id.bytesize } + retained_data_bytes(entry.status_data)
    end
  end

  def latest_leaf_id
    entries.each_with_object({ leaf_id: nil }) do |entry, state|
      next unless entry.id && entry.type != "session"

      state[:leaf_id] = entry.type == "leaf" ? entry.target_id : entry.id
    end.fetch(:leaf_id)
  end

  def entries_for_leaf(leaf_id, supplied:)
    return entries unless supplied
    return entries.select { |entry| entry.type == "session" || entry.id.nil? } if leaf_id.nil?

    path_ids = {}
    entry = by_id[leaf_id]
    while entry && !path_ids.key?(entry.id)
      path_ids[entry.id] = true
      entry = by_id[entry.parent_id]
    end
    return if path_ids.empty?

    entries.select { |candidate| candidate.id.nil? || path_ids[candidate.id] }
  end

  def scan_metadata(file, entry)
    scanner = PiSessionIndexScanner.new
    remaining = entry.length
    file.seek(entry.offset)
    while remaining.positive?
      chunk = file.read([SCAN_CHUNK_BYTES, remaining].min)
      return unless chunk

      scanner.feed(chunk)
      remaining -= chunk.bytesize
    end
    metadata = scanner.finish
    return unless metadata

    metadata[:segments] = metadata[:segments].map { |segment| Segment.new(**segment) }
    metadata
  end

  private

  def retained_data_bytes(value)
    case value
    when Hash
      80 + value.sum { |key, item| retained_data_bytes(key) + retained_data_bytes(item) }
    when Array
      40 + value.sum { |item| retained_data_bytes(item) }
    when String
      40 + value.bytesize
    else
      16
    end
  end

  def extension_offset(previous, stat, file, digest)
    return 0 unless previous
    unless previous.device == stat.dev && previous.inode == stat.ino && stat.size > previous.size && previous.complete && previous.ends_with_newline
      raise Errno::EAGAIN
    end

    remaining = previous.size
    file.rewind
    while remaining.positive?
      chunk = file.read([SCAN_CHUNK_BYTES, remaining].min)
      raise Errno::EAGAIN unless chunk

      digest.update(chunk)
      remaining -= chunk.bytesize
    end
    raise Errno::EAGAIN unless digest.hexdigest == previous.fingerprint

    previous.size
  end

  def scan(file, start_offset, digest)
    offset = start_offset
    line_offset = start_offset
    line_length = 0
    materialized = +""
    materializing = true
    scanner = nil

    read_anything = false
    while (chunk = file.gets("\n", SCAN_CHUNK_BYTES))
      digest.update(chunk)
      read_anything = true
      @ends_with_newline = chunk.end_with?("\n")
      line_length += chunk.bytesize
      if materializing
        if line_length <= MATERIALIZED_ENTRY_BYTES
          materialized << chunk
        else
          prefix = materialized + chunk
          scanner = fast_large_metadata(prefix) || PiSessionIndexScanner.new
          unless scanner.is_a?(Hash)
            scanner.feed(prefix)
          end
          materialized.clear
          materializing = false
        end
      elsif !scanner.is_a?(Hash)
        scanner.feed(chunk)
      end

      offset += chunk.bytesize
      next unless chunk.end_with?("\n")

      appended = append_entry(line_offset, line_length, materializing ? materialized : nil, scanner)
      @supported = false unless appended || materializing
      line_offset = offset
      line_length = 0
      materialized = +""
      materializing = true
      scanner = nil
    end

    if line_length.positive?
      appended = append_entry(line_offset, line_length, materializing ? materialized : nil, scanner)
      @supported = false unless appended || materializing
      @complete = false unless appended
    elsif read_anything
      @complete = true
    end
  end

  def append_entry(offset, length, materialized, scanner)
    metadata = if materialized
      parsed_metadata(materialized)
    else
      native_large_metadata(scanner)
    end
    return false unless metadata

    entry = Entry.new(**metadata, ordinal: entries.length, offset: offset, length: length)
    entries << entry
    by_id[entry.id] = entry if entry.id
    true
  end

  def parsed_metadata(line)
    return if line.strip.empty?

    entry = JSON.parse(line)
    @metadata_from_entry.call(entry)
  rescue JSON::ParserError
    nil
  end

  def native_large_metadata(scanner)
    metadata = scanner.is_a?(Hash) ? scanner : scanner.finish
    return unless metadata

    metadata[:segments] = metadata[:segments].map { |segment| Segment.new(**segment) }
    metadata
  end

  def fast_large_metadata(prefix)
    match = prefix.match(/\A\{"type":"message","id":"([^"\\]+)","parentId":(null|"[^"\\]+"),"timestamp":"[^"\\]+","message":\{"role":"(user|toolResult)"/)
    return unless match

    return if match[1].bytesize > PiSessionIndexScanner::CAPTURE_BYTES

    role = match[3]
    tool_call_id = nil
    tool_name = nil
    if role == "toolResult"
      tool_match = prefix.match(/\A.*?"role":"toolResult","toolCallId":"([^"\\]+)","toolName":"([^"\\]+)","content":/m)
      return unless tool_match

      tool_call_id = tool_match[1]
      tool_name = tool_match[2]
      return if tool_call_id.bytesize > PiSessionIndexScanner::CAPTURE_BYTES || tool_name.bytesize > PiSessionIndexScanner::CAPTURE_BYTES
      return unless prefix.match?(/"content":\[\{"type":"text","text":"(?!")/)
    end

    {
      type: "message",
      id: match[1],
      parent_id: match[2] == "null" ? nil : match[2][1...-1],
      target_id: nil,
      role: role,
      segments: [{ role: role, tool_call_id: tool_call_id, tool_name: tool_name, minimum_window_bytes: :deferred, paired_minimum_window_bytes: :deferred }],
      subagent_tool_call_ids: [],
      status_data: nil,
      estimate_text_length: nil,
      deferred_metadata: true
    }
  end

  def same_stat?(left, right)
    left.dev == right.dev && left.ino == right.ino && left.size == right.size && stat_mtime_ns(left) == stat_mtime_ns(right)
  end

  def stat_mtime_ns(stat)
    (stat.mtime.to_i * 1_000_000_000) + stat.mtime.nsec
  end
end
