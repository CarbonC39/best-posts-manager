package platform

import "testing"

func TestBridgePlatformCapabilitiesStayManual(t *testing.T) {
	for _, name := range []string{"x", "weibo", "xiaohongshu", "qzone"} {
		capability := CapabilityFor(name)
		if capability.Platform != name || capability.Format != "plain" || len(capability.Visibilities) != 1 || capability.Visibilities[0] != "public" {
			t.Fatalf("%s capability: %+v", name, capability)
		}
		if capability.VisibilityNote == "" || capability.CustomEmoji || capability.SupportsCW || capability.SupportsLocalOnly {
			t.Fatalf("%s must retain an explicit manual-check boundary: %+v", name, capability)
		}
	}
}
