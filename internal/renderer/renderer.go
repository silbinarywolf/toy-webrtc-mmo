package renderer

import (
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/renderer/internal/rendereriface"
)

// ImageOptions are draw options for an image
type ImageOptions = rendereriface.ImageOptions

// Image is a sprite loaded by the renderer
type Image = rendereriface.Image

type Screen = rendereriface.Screen

// App is the implementation of the renderer
type App = appImplementation // appImplementation changes type based on build tags, we do this so function calls are inlined and cost less, checked with "go build -gcflags=-m=2"
