require "minitest/autorun"
require_relative "../lib/prompts/bash_command"

class PromptsBashCommandTest < Minitest::Test
  def test_parses_included_and_excluded_commands
    included = Prompts::BashCommand.parse("!  pwd  ")
    excluded = Prompts::BashCommand.parse("!!  git status  ")

    assert_equal "pwd", included.command
    refute included.exclude_from_context
    assert_equal "git status", excluded.command
    assert excluded.exclude_from_context
  end

  def test_parses_multiline_commands
    command = Prompts::BashCommand.parse("!printf 'one\\ntwo'\n| cat")

    assert_equal "printf 'one\\ntwo'\n| cat", command.command
  end

  def test_only_recognizes_a_prefix_at_the_first_character
    assert_nil Prompts::BashCommand.parse(" !pwd")
    assert_nil Prompts::BashCommand.parse("\n!pwd")
    assert_nil Prompts::BashCommand.parse("Run !pwd")
  end

  def test_empty_commands_fall_through_to_regular_prompts
    assert_nil Prompts::BashCommand.parse("!")
    assert_nil Prompts::BashCommand.parse("!  \n\t")
    assert_nil Prompts::BashCommand.parse("!!")
    assert_nil Prompts::BashCommand.parse("!!  ")
  end
end
