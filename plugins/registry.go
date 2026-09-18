package plugins

import (
	"gaia/kernel"
	"gaia/plugins/agent"
	"gaia/plugins/ask"
	"gaia/plugins/cache"
	"gaia/plugins/chat"
	configplugin "gaia/plugins/config"
	"gaia/plugins/investigate"
	"gaia/plugins/mempalace"
	"gaia/plugins/models"
	"gaia/plugins/roles"
	"gaia/plugins/sanitize"
	"gaia/plugins/serve"
	"gaia/plugins/version"
)

// RegisterAll registers all built-in plugins with the kernel.
func RegisterAll(k *kernel.Kernel) error {
	if err := k.RegisterPlugin(agent.NewAgentPlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(ask.NewAskPlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(chat.NewChatPlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(configplugin.NewConfigPlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(cache.NewCachePlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(NewPluginsPlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(version.NewVersionPlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(investigate.NewInvestigatePlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(models.NewModelsPlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(mempalace.NewMemPalacePlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(roles.NewRolesPlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(sanitize.NewSanitizerPlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(serve.NewServePlugin()); err != nil {
		return err
	}
	return nil
}
