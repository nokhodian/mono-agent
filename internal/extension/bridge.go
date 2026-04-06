package extension

import "github.com/nokhodian/mono-agent/internal/browser"

// ServerBridge adapts *Server to satisfy browser.ExtensionBridge, breaking the
// import cycle between the browser and extension packages.
type ServerBridge struct {
	Server *Server
}

// Compile-time check.
var _ browser.ExtensionBridge = (*ServerBridge)(nil)

func (b *ServerBridge) IsConnected() bool {
	return b.Server.IsConnected()
}

func (b *ServerBridge) CreateTab(url string) (int, error) {
	return b.Server.CreateTab(url)
}

func (b *ServerBridge) NewPage(tabID int) browser.PageInterface {
	return NewExtensionPage(b.Server, tabID)
}
