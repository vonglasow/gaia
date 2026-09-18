Feature: The command line

  What a person typing `gaia` gets back before any model is ever contacted.
  Everything here runs offline: no provider is reachable from a test, and
  nothing in this file needs one.

  Background:
    Given a home of my own

  Rule: The tool says what it can do

    Scenario: The bare command lists its commands
      When I run "--help"
      Then it exits with code 0
      And the output contains "Available Commands:"
      And the output contains "plugins"
      And the output contains "config"

    Scenario: The version is reported, stamps and all
      When I run "version"
      Then it exits with code 0
      And the output contains "Gaia"
      And the output contains "commit"
      And nothing is written to standard error

  Rule: A command nobody wrote is refused, and the shell can tell

    A wrong command name is the one error path gaia reports through cobra
    rather than through its own printer, which is why the exit code here is
    the one a script would act on.

    Scenario: An unknown command fails loudly
      When I run "nosuchcommand"
      Then it exits with code 1
      And standard error contains "unknown command"

    Scenario: An unknown subcommand of a real command fails too
      When I run "config nosuchsubcommand"
      Then it exits with code 1
      And standard error contains "unknown command"

  Rule: A failure the person can read is a failure the shell can act on

    Every one of these used to print its reason and exit 0, so a script under
    `set -e` walked straight past them and `gaia ... && echo ok` printed both
    lines. The reason goes to standard error; the exit code says it failed.

    Scenario: Reading a key nobody set fails
      When I run "config get ask.model"
      Then it exits with code 1
      And standard error contains "is not set"

    Scenario: Enabling a plugin nobody wrote fails
      When I run "plugins enable nosuchplugin"
      Then it exits with code 1
      And standard error contains "Unknown plugin"

    Scenario: A reason is never printed twice
      When I run "config get ask.model"
      Then it exits with code 1
      And standard error contains "is not set"
      And standard error does not contain "reported"
