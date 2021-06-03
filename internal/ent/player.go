// ent is the entity package
package ent

import (
	"image"
	"strings"

	"github.com/silbinarywolf/toy-webrtc-mmo/internal/asset"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/renderer"
)

var (
	leftSprite  renderer.Image
	rightSprite renderer.Image
)

func LoadPlayerAssets(app renderer.App) {
	img, _, err := image.Decode(strings.NewReader(asset.PlayerIdleRight))
	if err != nil {
		panic(err)
	}
	rightSprite = app.NewImageFromImage(img)

	img, _, err = image.Decode(strings.NewReader(asset.PlayerIdleLeft))
	if err != nil {
		panic(err)
	}
	leftSprite = app.NewImageFromImage(img)
}

type Player struct {
	// NetID is the unique ID of the player on the network
	NetID uint16
	// Inputs are the buttons/keys used by the player to make them move/etc
	Inputs         PlayerInput
	X, Y           float32
	Width, Height  float32
	Hspeed, Vspeed float32
	DirLeft        bool
}

type PlayerInput struct {
	IsHoldingLeft  bool
	IsHoldingRight bool
	IsHoldingJump  bool
}

func (self *Player) Init() {
	self.Width = 96
	self.Height = 96
}

func (self *Player) Update() {
	var (
		groundY float32 = 720 - self.Height
	)
	const (
		// maxSpeed is the maximum horizontal speed for the entity
		maxSpeed float32 = 8
		// lubrication is how quickly an entity speeds up if holding down a key per frame
		lubrication float32 = 4
		// fricition is how quickly an entity slows down (ie. ice/slippy)
		friction float32 = 2
		// jumpPower is the initial vspeed to jump at
		jumpPower float32 = 12
		// gravity is applied to vspeed per step
		gravity float32 = 0.45
	)

	// Update via input
	if self.Inputs.IsHoldingLeft {
		if self.Hspeed > 0 {
			self.Hspeed = 0
		}
		self.Hspeed -= lubrication
		self.DirLeft = true
	} else if self.Inputs.IsHoldingRight {
		if self.Hspeed < 0 {
			self.Hspeed = 0
		}
		self.Hspeed += lubrication
		self.DirLeft = false
	}
	if self.Inputs.IsHoldingJump &&
		self.Vspeed >= 0 &&
		self.Y+self.Height+1 > groundY {
		self.Vspeed = -jumpPower
	}

	// Update lubrication/friction (X axis)
	if self.Hspeed > 0 {
		self.Hspeed -= friction
		if self.Hspeed < 0 {
			self.Hspeed = 0
		}
		// Cap to max speed
		if self.Hspeed > maxSpeed {
			self.Hspeed = maxSpeed
		}
	}
	if self.Hspeed < 0 {
		self.Hspeed += friction
		if self.Hspeed > 0 {
			self.Hspeed = 0
		}
		// Cap to max speed
		if self.Hspeed < -maxSpeed {
			self.Hspeed = -maxSpeed
		}
	}
	self.X += self.Hspeed

	// Update gravity (Y axis)
	self.Vspeed += gravity
	if self.Vspeed > 20 {
		// cap fall speed
		self.Vspeed = 20
	}
	self.Y += self.Vspeed
	if self.Y+self.Height > groundY {
		self.Y = groundY - self.Height
	}
}

func (self *Player) Draw(screen renderer.Screen) {
	s := rightSprite
	if self.DirLeft {
		s = leftSprite
	}

	screen.DrawImage(s, renderer.ImageOptions{
		X: self.X,
		Y: self.Y,
	})
}
