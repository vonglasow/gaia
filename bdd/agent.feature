Feature: Working on a project with a model

  `gaia agent` puts a model in front of a repository. Nothing here contacts one:
  these are the decisions gaia makes before a model is ever asked anything, which
  is where the guarantees live.

  Background:
    Given a home of my own

  Rule: A run says what it is about to do

    Scenario: The command explains itself
      When I run "agent --help"
      Then it exits with code 0
      And the output contains "reads files"
      And the output contains "--write"

  Rule: Nothing happens without somewhere to work and something to work with

    Both are checked before a model is asked anything, so a mistake at the
    command line costs nothing.

    Scenario: A project directory that is not there stops the run
      Given a configuration file containing:
        """
        model: "a-model"
        """
      When I run "agent -C /nowhere/at/all describe this project"
      Then it exits with code 1
      And standard error contains "no such file"

    Scenario: With no model configured anywhere it says so
      When I run "agent describe this project"
      Then it exits with code 1
      And standard error contains "no model configured"

  Rule: Writing is something you ask for

    The default is an agent that reads, runs the project's checks and explains.
    That is the useful thing that needs nobody's decision; changing files is the
    part somebody has to mean.

    Scenario: By default it may not change anything
      When I run "agent --help"
      Then it exits with code 0
      And the output contains "off by default"

  Rule: The model is chosen where the work is

    A 7B answers a question about a file and a 30B writes the patch, so trying
    one must not mean editing a file.

    Scenario: The command takes a model and a provider
      When I run "agent --help"
      Then it exits with code 0
      And the output contains "--model"
      And the output contains "--provider"

    Scenario: So does investigate
      When I run "investigate --help"
      Then it exits with code 0
      And the output contains "--model"
      And the output contains "--provider"
