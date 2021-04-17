package asset

import (
	_ "embed"
	_ "image/png"
)

//go:embed player_idle_right.png
var PlayerIdleRight string

//go:embed player_idle_left.png
var PlayerIdleLeft string

//go:embed background.png
var Background string
