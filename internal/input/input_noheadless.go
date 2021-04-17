// +build !headless

package input

import "github.com/hajimehoshi/ebiten/v2"

func isKeyPressed(key Key) bool {
	return ebiten.IsKeyPressed(ebiten.Key(key))
}

func isMouseButtonPressed(mouseButton MouseButton) bool {
	return ebiten.IsMouseButtonPressed(ebiten.MouseButton(mouseButton))
}

func mousePosition() (int, int) {
	x, y := ebiten.CursorPosition()
	return x, y
}

func touchIDs() []TouchID {
	touchIDs := ebiten.TouchIDs()
	if len(touchIDs) == 0 {
		return nil
	}
	r := make([]TouchID, len(touchIDs))
	for i, touchID := range touchIDs {
		r[i] = TouchID(touchID)
	}
	return r
}

func touchPosition(touchID TouchID) (int, int) {
	x, y := ebiten.TouchPosition(ebiten.TouchID(touchID))
	return x, y
}
