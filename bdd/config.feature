Feature: Configuration

  Where gaia's settings come from, which keys it will accept, and what it does
  when it is handed one nobody declared.

  Background:
    Given a home of my own

  Rule: The file in use is discoverable

    Scenario: The path is the one under the home in use
      When I run "config path"
      Then it exits with code 0
      And the output contains ".config/gaia/config.yaml"

    Scenario: The file is created on demand
      When I run "config create"
      Then it exits with code 0
      And the output contains "config.yaml"

    Scenario: A file handed over on the command line wins over the home
      Given a configuration file containing:
        """
        ask:
          model: from-the-flag
        """
      When I run "config get ask.model"
      Then it exits with code 0
      And the output contains "from-the-flag"

  Rule: A key set is a key read back

    Scenario: Writing then reading a plugin key
      When I run "config set ask.model llama3.1"
      And I run "config get ask.model"
      Then it exits with code 0
      And the output contains "llama3.1"

    Scenario: Listing shows the keys plugins declared
      When I run "config list"
      Then it exits with code 0
      And the output contains "config.validation"
      And the output contains "cache.enabled"

  Rule: A key no plugin declared is not a key

    The schema is what every plugin promises about its own namespace. A key
    outside it is a typo the person will otherwise spend an afternoon on,
    watching a setting they believe they wrote have no effect at all.

    Scenario: Setting an undeclared key is refused
      When I run "config set bogus.key value"
      Then it exits with code 1
      And standard error contains "invalid config key"

    Scenario: Under strict validation an undeclared key stops the command
      Given a configuration file containing:
        """
        config:
          validation: strict
        bogus: 1
        """
      When I run "version"
      Then it exits with code 1
      And standard error contains "invalid config keys"

    Scenario: Under warn validation the same key lets the command through
      Given a configuration file containing:
        """
        config:
          validation: warn
        bogus: 1
        """
      When I run "version"
      Then it exits with code 0
      And the output contains "Gaia"

    Scenario: With validation off the key is not even mentioned
      Given a configuration file containing:
        """
        config:
          validation: "off"
        bogus: 1
        """
      When I run "version"
      Then it exits with code 0
      And the output contains "Gaia"
