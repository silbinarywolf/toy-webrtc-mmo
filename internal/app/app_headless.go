// +build headless

package app

import (
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/renderer/headless"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/renderer/renderer"
)

func getRenderDriver() renderer.App {
	return new(headless.App)
}
