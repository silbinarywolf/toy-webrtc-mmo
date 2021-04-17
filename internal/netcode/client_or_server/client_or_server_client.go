// +build !server

package client_or_server

import (
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/client"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/netconf"
)

// NewClientOrServer will return client for non-server tagged builds
func NewClientOrServer(options netconf.Options) netcode.Controller {
	return client.New(options)
}
