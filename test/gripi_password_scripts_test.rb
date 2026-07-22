require "fileutils"
require "minitest/autorun"
require "open3"
require "tmpdir"

class GripiPasswordScriptsTest < Minitest::Test
  def test_password_helper_generates_password_once_and_prints_change_warning
    ensure_go_binary
    Dir.mktmpdir do |dir|
      env_path = File.join(dir, "gateway-env")
      env = { "GRIPI_ENV_PATH" => env_path }

      first_stdout, first_stderr, first_status = Open3.capture3(env, "bin/gripi-password", chdir: repo_root)

      assert first_status.success?, first_stderr
      password = File.read(env_path).match(/\AGRIPI_ADMIN_PASSWORD=([0-9a-f]{24})\n\z/)[1]
      assert_includes first_stdout, "Generated GRIPI_ADMIN_PASSWORD in #{env_path}"
      assert_includes first_stdout, "Admin password: #{password}"
      assert_includes first_stdout, "You should change it by editing #{env_path}"

      second_stdout, second_stderr, second_status = Open3.capture3(env, "bin/gripi-password", chdir: repo_root)

      assert second_status.success?, second_stderr
      assert_empty second_stdout
      refute_includes second_stdout, password
    end
  end

  def test_password_helper_preserves_existing_bytes_and_secures_permissions
    ensure_go_binary
    Dir.mktmpdir do |dir|
      env_path = File.join(dir, "gateway-env")
      original = "EXISTING=value without newline"
      File.binwrite(env_path, original)
      File.chmod(0o644, env_path)

      stdout, stderr, status = Open3.capture3({ "GRIPI_ENV_PATH" => env_path }, "bin/gripi-password", chdir: repo_root)

      assert status.success?, stderr
      assert_match(/Admin password: [0-9a-f]{24}/, stdout)
      assert_match(/\A#{Regexp.escape(original)}\nGRIPI_ADMIN_PASSWORD=[0-9a-f]{24}\n\z/, File.binread(env_path))
      assert_equal 0o600, File.stat(env_path).mode & 0o777
    end
  end

  def test_password_setup_reuses_a_persistent_lock_file
    ensure_go_binary
    Dir.mktmpdir do |dir|
      env_path = File.join(dir, "gateway-env")
      File.write("#{env_path}.lock", "")

      _stdout, stderr, status = Open3.capture3({ "GRIPI_ENV_PATH" => env_path }, "bin/gripi-password", chdir: repo_root)

      assert status.success?, stderr
      assert_match(/\AGRIPI_ADMIN_PASSWORD=[0-9a-f]{24}\n\z/, File.binread(env_path))
    end
  end

  def test_concurrent_password_setup_selects_exactly_one_authoritative_password
    ensure_go_binary
    Dir.mktmpdir do |dir|
      env_path = File.join(dir, "gateway-env")
      outputs = Array.new(12) do
        Thread.new { Open3.capture3({ "GRIPI_ENV_PATH" => env_path }, "bin/gripi-password", chdir: repo_root) }
      end.map(&:value)

      assert outputs.all? { |_stdout, _stderr, status| status.success? }, outputs.map { |_, stderr, _| stderr }.join
      generated = outputs.filter_map { |stdout, _stderr, _status| stdout[/Admin password: ([0-9a-f]{24})/, 1] }
      passwords = File.binread(env_path).scan(/^GRIPI_ADMIN_PASSWORD=([0-9a-f]{24})$/).flatten
      assert_equal 1, generated.length
      assert_equal generated, passwords
      assert_equal 0o600, File.stat(env_path).mode & 0o777
    end
  end

  def test_password_helper_does_not_rewrite_a_file_that_already_has_a_password
    ensure_go_binary
    Dir.mktmpdir do |dir|
      env_path = File.join(dir, "gateway-env")
      original = "# byte-exact\nGRIPI_ADMIN_PASSWORD=already-set\nTAIL=no-newline"
      File.binwrite(env_path, original)

      stdout, stderr, status = Open3.capture3({ "GRIPI_ENV_PATH" => env_path }, "bin/gripi-password", chdir: repo_root)

      assert status.success?, stderr
      assert_empty stdout
      assert_equal original, File.binread(env_path)
      assert_equal 0o600, File.stat(env_path).mode & 0o777
    end
  end

  def test_setup_installs_node_dependencies_builds_go_and_invokes_the_built_password_helper
    Dir.mktmpdir do |dir|
      project = File.join(dir, "project")
      project_bin = File.join(project, "bin")
      fake_bin = File.join(dir, "fake-bin")
      calls = File.join(dir, "mise-calls")
      env_path = File.join(dir, "gripi-env")
      FileUtils.mkdir_p([project_bin, fake_bin])
      FileUtils.cp(File.join(repo_root, "bin/setup"), project_bin)
      FileUtils.cp(File.join(repo_root, "bin/gripi-password"), project_bin)
      File.write(File.join(fake_bin, "mise"), <<~SH)
        #!/bin/sh
        printf '%s\n' "$*" >> "$MISE_CALLS"
        if [ "$*" = "exec -- go build -o tmp/gripi ./cmd/gripi" ]; then
          mkdir -p tmp
          cat > tmp/gripi <<'BINARY'
        #!/bin/sh
        [ "$1" = password ] || exit 2
        printf 'GRIPI_ADMIN_PASSWORD=0123456789abcdef01234567\n' > "$GRIPI_ENV_PATH"
        BINARY
          chmod +x tmp/gripi
        fi
      SH
      File.chmod(0o755, File.join(fake_bin, "mise"))

      env = {
        "PATH" => "#{fake_bin}:#{ENV.fetch("PATH")}",
        "MISE_CALLS" => calls,
        "GRIPI_ENV_PATH" => env_path
      }
      _stdout, stderr, status = Open3.capture3(env, File.join(project_bin, "setup"), chdir: project)

      assert status.success?, stderr
      assert_equal ["exec -- npm ci", "exec -- go build -o tmp/gripi ./cmd/gripi"], File.readlines(calls, chomp: true)
      assert_equal "GRIPI_ADMIN_PASSWORD=0123456789abcdef01234567\n", File.read(env_path)
    end
  end

  private

  def ensure_go_binary
    _stdout, stderr, status = Open3.capture3("mise", "exec", "--", "go", "build", "-o", "tmp/gripi", "./cmd/gripi", chdir: repo_root)
    assert status.success?, stderr
  end

  def repo_root
    File.expand_path("..", __dir__)
  end
end
