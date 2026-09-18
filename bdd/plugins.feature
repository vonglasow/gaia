Feature: Plugins

  Every feature gaia has is a plugin, and which ones are live is a matter of
  configuration. This file is about what that switch actually does — not that
  a listing changes wording, but that the command line itself changes shape.

  Background:
    Given a home of my own

  Rule: The listing reports every built-in plugin and its state

    Scenario: The built-ins are all there
      When I run "plugins list"
      Then it exits with code 0
      And the output contains "ask"
      And the output contains "config"
      And the output contains "roles"
      And the output contains "agent"

    Scenario: The listing says which are on and what their default is
      When I run "plugins list"
      Then it exits with code 0
      And the output contains "default=true"

  Rule: Disabling a plugin removes its commands, not just its line in a listing

    This is the scenario that makes the switch worth having. A disabled plugin
    that still answers is a plugin that was never really disabled, and nothing
    in a listing would have told you.

    Scenario: A disabled plugin's command is gone from the CLI
      Given a configuration file containing:
        """
        plugins:
          disabled: ["roles"]
        """
      When I run "roles list"
      Then it exits with code 1
      And standard error contains "unknown command"

    Scenario: The rest of the CLI is unaffected by one plugin being off
      Given a configuration file containing:
        """
        plugins:
          disabled: ["roles"]
        """
      When I run "plugins list"
      Then it exits with code 0
      And the output contains "ask"

    Scenario: A plugin named in enabled is on whatever else is configured
      Given a configuration file containing:
        """
        plugins:
          enabled: ["chat"]
        """
      When I run "chat --help"
      Then it exits with code 0
