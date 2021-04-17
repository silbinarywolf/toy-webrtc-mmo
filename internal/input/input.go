package input

// Key represents a keyboard key.
type Key int32

// Only defining keys used by this game
//
// We indirectly use ebiten constants so that ebiten isn't included
// as a package for headless builds
const (
	KeyA     = Key(10)  // Key(ebiten.KeyA)
	KeyD     = Key(13)  // Key(ebiten.KeyD)
	KeyLeft  = Key(79)  // Key(ebiten.KeyLeft)
	KeyRight = Key(92)  // Key(ebiten.KeyRight)
	KeySpace = Key(100) // Key(ebiten.KeySpace)
)

func IsKeyPressed(key Key) bool {
	return isKeyPressed(key)
}

// MouseButton represents a mouse button (left, right or middle)
type MouseButton int32

// Define all mouse buttons as there are only 3.
//
// We indirectly use ebiten constants so that ebiten isn't included
// as a package for headless builds
const (
	MouseButtonLeft   = MouseButton(0)
	MouseButtonRight  = MouseButton(1)
	MouseButtonMiddle = MouseButton(2)
)

func IsMouseButtonPressed(mouseButton MouseButton) bool {
	return isMouseButtonPressed(mouseButton)
}

// MousePosition returns the mouse/cursor position
//
// For headless builds, this always returns (0,0)
func MousePosition() (int, int) {
	x, y := mousePosition()
	return x, y
}

type TouchID int

func TouchIDs() []TouchID {
	return touchIDs()
}

func TouchPosition(touchID TouchID) (int, int) {
	x, y := touchPosition(touchID)
	return x, y
}
