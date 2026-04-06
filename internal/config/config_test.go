package config

import "testing"

func TestDefaultConfig_PeerTemplateName(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Core.PeerTemplateName != "Peer {{.Random}}" {
		t.Errorf("expected PeerTemplateName %q, got %q", "Peer {{.Random}}", cfg.Core.PeerTemplateName)
	}
}
