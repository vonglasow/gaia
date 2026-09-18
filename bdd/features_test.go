package bdd

import "testing"

// One Go test per feature file.

func TestCommandLine(t *testing.T) { runFeature(t, "cli.feature") }

func TestConfiguration(t *testing.T) { runFeature(t, "config.feature") }

func TestPlugins(t *testing.T) { runFeature(t, "plugins.feature") }

func TestRoles(t *testing.T) { runFeature(t, "roles.feature") }

func TestAgent(t *testing.T) { runFeature(t, "agent.feature") }
