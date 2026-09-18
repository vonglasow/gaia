package ask

// NoModelsInstalled holds Ollama still for a test: nothing loaded, nothing
// installed, so "no model configured" stays a fact rather than a machine's mood.
func NoModelsInstalled(t interface{ Cleanup(func()) }) {
	previous := InstalledModels
	InstalledModels = func(string, int) ([]string, []string) { return nil, nil }
	t.Cleanup(func() { InstalledModels = previous })
}

// ModelsInstalled makes a test see exactly these, none of them loaded.
func ModelsInstalled(t interface{ Cleanup(func()) }, names ...string) {
	previous := InstalledModels
	InstalledModels = func(string, int) ([]string, []string) { return nil, names }
	t.Cleanup(func() { InstalledModels = previous })
}

// ModelLoaded makes a test see one model already in memory.
func ModelLoaded(t interface{ Cleanup(func()) }, name string) {
	previous := InstalledModels
	InstalledModels = func(string, int) ([]string, []string) {
		return []string{name}, []string{name}
	}
	t.Cleanup(func() { InstalledModels = previous })
}
