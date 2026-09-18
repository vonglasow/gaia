Feature: Roles

  A role is a system prompt kept in a YAML file rather than retyped into every
  question. What this file pins down is the reading: which directory is
  consulted, what a role has to carry to count as one, and what comes back
  when the prompt is asked for.

  Background:
    Given a home of my own

  Rule: Roles are read from the configured directory and nowhere else

    Scenario: A role in the directory is listed
      Given a roles directory holding "pirate":
        """
        name: pirate
        description: Answers like a pirate
        priority: 10
        enabled: true
        system_prompt: |
          Answer like a pirate.
        """
      When I run "roles list"
      Then it exits with code 0
      And the output contains "pirate"

    # Deliberately not asserted here: every role file in this repository
    # carries an `enabled:` key, and nothing in plugins/roles reads it — a
    # role set to false is still listed. Pinning that down as a scenario would
    # turn a finding into a promise.

  Rule: The prompt a role carries can be read back whole

    Showing the prompt is what makes a role reviewable. A role whose text can
    only be observed by its effect on a model's answer is a prompt nobody can
    diff.

    Scenario: Showing a role prints its name and its prompt
      Given a roles directory holding "pirate":
        """
        name: pirate
        description: Answers like a pirate
        priority: 10
        enabled: true
        system_prompt: |
          Answer like a pirate.
        """
      When I run "roles show pirate"
      Then it exits with code 0
      And the output contains "pirate"
      And the output contains "Answer like a pirate."
