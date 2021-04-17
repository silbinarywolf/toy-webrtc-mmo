// +build headless

package input

func isKeyPressed(key Key) bool {
	return false
}

func isMouseButtonPressed(mouseButton MouseButton) bool {
	return false
}

func mousePosition() (int, int) {
	return 0, 0
}

func touchIDs() []TouchID {
	return nil
}

func touchPosition(touchID TouchID) (int, int) {
	return 0, 0
}
