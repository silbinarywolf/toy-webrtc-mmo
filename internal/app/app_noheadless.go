// +build !headless

package app

import (
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/renderer/ebiten"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/renderer/renderer"
)

func getRenderDriver() renderer.App {
	return new(ebiten.App)
}
