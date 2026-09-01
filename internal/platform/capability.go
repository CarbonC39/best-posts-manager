package platform

// PlatformCapability is the small, UI-facing contract shared by supported
// adapters. It covers only first-version controls and validation.
type PlatformCapability struct {
	Platform          string   `json:"platform"`
	Format            string   `json:"format"`
	TextMaxBytes      int      `json:"text_max_bytes"`
	MaxMedia          int      `json:"max_media"`
	MaxMediaBytes     int64    `json:"max_media_bytes"`
	Visibilities      []string `json:"visibilities"`
	SupportsCW        bool     `json:"supports_cw"`
	SupportsLocalOnly bool     `json:"supports_local_only"`
	CustomEmoji       bool     `json:"custom_emoji"`
	VisibilityNote    string   `json:"visibility_note"`
}

func CapabilityFor(name string) PlatformCapability {
	switch name {
	case "misskey":
		return PlatformCapability{Platform: name, Format: "mfm", MaxMedia: 4, MaxMediaBytes: 32 << 20, Visibilities: []string{"public", "home", "followers"}, SupportsCW: true, SupportsLocalOnly: true, CustomEmoji: true}
	case "bluesky":
		return PlatformCapability{Platform: name, Format: "plain", TextMaxBytes: 3000, MaxMedia: 4, MaxMediaBytes: 1_000_000, Visibilities: []string{"public"}}
	case "mastodon", "mastodon-compatible":
		return PlatformCapability{Platform: name, Format: "plain", MaxMedia: 4, MaxMediaBytes: 32 << 20, Visibilities: []string{"public", "unlisted", "private", "direct"}, SupportsCW: true, CustomEmoji: true}
	case "x", "weibo", "xiaohongshu", "qzone":
		return PlatformCapability{Platform: name, Format: "plain", Visibilities: []string{"public"}, VisibilityNote: "由平台页面控制，程序不假定数量上限；请人工检查可见范围与图片顺序"}
	default:
		return PlatformCapability{Platform: name}
	}
}
