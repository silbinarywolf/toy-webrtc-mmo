// headless is the headless mode driver for the game so we can avoid building the
// ebiten library into the server binary
package headless

import (
	"image"
	"time"

	"github.com/silbinarywolf/toy-webrtc-mmo/internal/renderer/renderer"
)

var _ renderer.App = new(App)

type App struct {
}

func (app *App) SetRunnableOnUnfocused(v bool) {
	// n/a for headless
}

func (app *App) SetWindowSize(width, height int) {
	// n/a for headless
}

func (app *App) SetWindowTitle(title string) {
	// n/a for headless
}

func (app *App) RunGame(game renderer.Game) error {
	// note(jae): 2021-03-18
	// this should probably align with how the Ebiten clock works
	// but I'm going to take a lazy shortcut.
	tick := time.NewTicker(16 * time.Millisecond)
	for {
		select {
		case <-tick.C:
			if err := game.Update(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (app *App) NewImageFromImage(img image.Image) renderer.Image {
	// n/a for headless
	return nil
}

type Screen struct {
}
