// +build server

package client_or_server

import (
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/netconf"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/server"
)

// NewClientOrServer will return server for server tagged builds
func NewClientOrServer(options netconf.Options) netcode.Controller {
	return server.New(options)
}
