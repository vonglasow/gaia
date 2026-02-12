package commands

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStringSlice_NilKey(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	out := getStringSlice("nonexistent.key")
	assert.Nil(t, out)
}

func TestGetStringSlice_InvalidTypeReturnsNil(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	viper.Set("operator.denylist", "not-a-list")
	out := getStringSlice("operator.denylist")
	assert.Nil(t, out)
}

func TestGetStringSlice_ValidSlice(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	viper.Set("operator.denylist", []string{"rm -rf", "sudo"})
	out := getStringSlice("operator.denylist")
	require.NotNil(t, out)
	assert.Equal(t, []string{"rm -rf", "sudo"}, out)
}

func TestGetStringSlice_InterfaceSlice(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	viper.Set("operator.denylist", []interface{}{"a", "b"})
	out := getStringSlice("operator.denylist")
	require.NotNil(t, out)
	assert.Equal(t, []string{"a", "b"}, out)
}

func TestDefaultOperatorDenylist(t *testing.T) {
	// When getStringSlice returns nil, runInvestigate uses defaultOperatorDenylist.
	// Ensure it matches config defaults and is non-empty for safety.
	assert.NotEmpty(t, defaultOperatorDenylist)
	assert.Contains(t, defaultOperatorDenylist, "rm -rf")
	assert.Contains(t, defaultOperatorDenylist, "sudo")
	assert.Contains(t, defaultOperatorDenylist, "mkfs")
	assert.Contains(t, defaultOperatorDenylist, "> /dev/sd")
}
