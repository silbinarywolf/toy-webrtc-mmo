package rendereriface

import "image"

type ImageOptions struct {
	X, Y           float32
	ScaleX, ScaleY float32
}

type Image interface {
}

// Game interface was copy-pasted out of Ebiten
type Game interface {
	Update() error
	Draw(screen Screen)
	Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int)
}

type App interface {
	SetRunnableOnUnfocused(v bool)
	SetWindowSize(screenWidth, screenHeight int)
	SetWindowTitle(title string)
	RunGame(game Game) error
	NewImageFromImage(img image.Image) Image
}

type Screen interface {
	DrawImage(img Image, options ImageOptions)
}
