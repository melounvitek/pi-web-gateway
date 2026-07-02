require "minitest/autorun"
require "tmpdir"
require "fileutils"
require_relative "../lib/configured_session_cwds"

class ConfiguredSessionCwdsTest < Minitest::Test
  def test_reads_accessible_directories_from_plain_text_file
    Dir.mktmpdir do |dir|
      first = File.join(dir, "first")
      second = File.join(dir, "second")
      missing = File.join(dir, "missing")
      FileUtils.mkdir_p(first)
      FileUtils.mkdir_p(second)
      config_path = File.join(dir, "session-cwds.txt")
      File.write(config_path, "\n# comment\n#{first}\n#{missing}\n#{second} # inline comment\n#{first}\n")

      assert_equal [File.realpath(first), File.realpath(second)], ConfiguredSessionCwds.read(config_path)
    end
  end

  def test_expands_home_directory
    Dir.mktmpdir do |home|
      project = File.join(home, "project")
      FileUtils.mkdir_p(project)
      config_path = File.join(home, "session-cwds.txt")
      File.write(config_path, "~/project\n")

      previous_home = ENV["HOME"]
      ENV["HOME"] = home
      assert_equal [File.realpath(project)], ConfiguredSessionCwds.read(config_path)
    ensure
      ENV["HOME"] = previous_home
    end
  end
end
