package sanitize

import "testing"

func TestSanitizerPluginSchema(t *testing.T) {
	p := NewSanitizerPlugin()
	keys := p.ConfigSchema()
	if len(keys) == 0 {
		t.Fatal("expected schema keys")
	}
}
