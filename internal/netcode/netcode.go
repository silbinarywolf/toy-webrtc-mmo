package netcode

import "github.com/silbinarywolf/toy-webrtc-mmo/internal/world"

type Controller interface {
	BeforeUpdate(world *world.World)
	HasStartedOrConnected() bool
}
