package ebiten

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/renderer/internal/rendereriface"
)

var _ rendereriface.App = new(App)

type App struct {
}

type ebitenGameAndScreen struct {
	rendereriface.Game
	screenDriver Screen
}

func (game *ebitenGameAndScreen) Draw(screen *ebiten.Image) {
	game.screenDriver.screen = screen
	game.Game.Draw(&game.screenDriver)
}

func (app *App) SetRunnableOnUnfocused(v bool) {
	ebiten.SetRunnableOnUnfocused(v)
}

func (app *App) SetWindowSize(width, height int) {
	ebiten.SetWindowSize(width, height)
}

func (app *App) SetWindowTitle(title string) {
	ebiten.SetWindowTitle(title)
}

func (app *App) NewImageFromImage(img image.Image) rendereriface.Image {
	return ebiten.NewImageFromImage(img)
}

func (app *App) RunGame(game rendereriface.Game) error {
	gameWrapper := ebitenGameAndScreen{}
	gameWrapper.Game = game
	return ebiten.RunGame(&gameWrapper)
}

type Screen struct {
	screen *ebiten.Image
}

var _ rendereriface.Screen = new(Screen)

func (driver *Screen) DrawImage(img rendereriface.Image, options rendereriface.ImageOptions) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(options.X), float64(options.Y))
	if options.ScaleX != 0 && options.ScaleY != 0 {
		op.GeoM.Scale(float64(options.ScaleX), float64(options.ScaleY))
	}
	driver.screen.DrawImage(img.(*ebiten.Image), op)
}
