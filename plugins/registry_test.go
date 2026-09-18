package plugins

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gaia/kernel"
)

// The registry is the only place in gaia that knows the concrete list of plugins.

func TestRegisterAllRegistersEveryBuiltInPlugin(t *testing.T) {
	k := kernel.NewKernel()
	require.NoError(t, RegisterAll(k))

	got := map[string]bool{}
	for _, p := range k.Plugins() {
		got[p.ID()] = true
	}

	for _, want := range []string{
		"agent", "ask", "cache", "chat", "config", "investigate", "mempalace",
		"models", "plugins", "roles", "sanitize", "serve", "version",
	} {
		require.True(t, got[want], "plugin %q is missing from the registry", want)
	}
}

// A plugin registered twice would have its schema and its commands attached twice over.
func TestRegisterAllRefusesToRunTwiceOnOneKernel(t *testing.T) {
	k := kernel.NewKernel()
	require.NoError(t, RegisterAll(k))
	require.Error(t, RegisterAll(k))
}

// Every key a plugin declares must sit in that plugin's own namespace.
func TestEveryPluginDeclaresKeysInItsOwnNamespace(t *testing.T) {
	k := kernel.NewKernel()
	require.NoError(t, RegisterAll(k))

	for _, p := range k.Plugins() {
		for _, key := range p.ConfigSchema() {
			require.Truef(t, len(key) > len(p.ID()) && key[:len(p.ID())+1] == p.ID()+".",
				"plugin %q declares %q, which is outside its namespace", p.ID(), key)
		}
	}
}

// A dependency naming a plugin that does not exist would only surface the day someone.
func TestEveryDeclaredDependencyExists(t *testing.T) {
	k := kernel.NewKernel()
	require.NoError(t, RegisterAll(k))

	known := map[string]bool{}
	for _, p := range k.Plugins() {
		known[p.ID()] = true
	}
	for _, p := range k.Plugins() {
		for _, dep := range p.DependsOn() {
			require.Truef(t, known[dep], "plugin %q depends on %q, which nothing registers", p.ID(), dep)
		}
	}
}

func TestThePluginsPluginDeclaresItself(t *testing.T) {
	p := NewPluginsPlugin()

	require.Equal(t, "plugins", p.ID())
	require.True(t, p.DefaultEnabled())
	require.Nil(t, p.DependsOn())
	require.Nil(t, p.MCPTools())
}

func TestUniqueAppendKeepsTheListSortedAndWithoutRepeats(t *testing.T) {
	require.Equal(t, []string{"ask", "chat"}, uniqueAppend([]string{"chat"}, "ask"))
	require.Equal(t, []string{"ask"}, uniqueAppend([]string{"ask"}, "ask"),
		"enabling what is already enabled changes nothing")
	require.Equal(t, []string{"ask"}, uniqueAppend([]string{"ask", "ask"}, ""),
		"an empty value adds nothing, and the duplicates already there are collapsed")
	require.Equal(t, []string{"ask"}, uniqueAppend(nil, "ask"))
}

func TestRemoveValueDropsOnlyWhatWasNamed(t *testing.T) {
	require.Equal(t, []string{"ask", "chat"}, removeValue([]string{"chat", "roles", "ask"}, "roles"))
	require.Equal(t, []string{"ask"}, removeValue([]string{"ask"}, "absent"))
	require.Equal(t, []string{}, removeValue([]string{"ask", "ask"}, "ask"),
		"every copy goes, not just the first")
}

// The list is written back into the config file as JSON.
func TestToJSONListQuotesEveryValue(t *testing.T) {
	require.Equal(t, `["ask","chat"]`, toJSONList([]string{"ask", "chat"}))
	require.Equal(t, `[]`, toJSONList(nil))
	require.Equal(t, `["a\"b"]`, toJSONList([]string{`a"b`}))
}
