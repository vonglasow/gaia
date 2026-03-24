package plugins

import "gaia/kernel"

// RegisterAll registers all built-in plugins with the kernel.
func RegisterAll(k *kernel.Kernel) error {
	if err := k.RegisterPlugin(NewAskPlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(NewConfigPlugin()); err != nil {
		return err
	}
	if err := k.RegisterPlugin(NewPluginsPlugin()); err != nil {
		return err
	}
	return nil
}
