package notify

type Capabilities struct {
	SendText          bool
	SendMarkdown      bool
	SendCard          bool
	UpdateCard        bool
	StreamCard        bool
	CardActions       bool
	NativeReply       bool
	Reactions         bool
	ReceiveEvents     bool
	DownloadResources bool
	Onboarding        bool
	NativeCardSchemas []string
}

func (c Capabilities) SupportsNativeCard(schema string) bool {
	for _, item := range c.NativeCardSchemas {
		if item == schema {
			return true
		}
	}
	return false
}

type PlatformCapabilities struct {
	NativeReply bool
	Reactions   bool
	SendCard    bool
	UpdateCard  bool
	CardActions bool
}
