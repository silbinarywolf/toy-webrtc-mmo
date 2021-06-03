package app

import (
	"image"
	"strings"

	"github.com/silbinarywolf/toy-webrtc-mmo/internal/asset"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/ent"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/input"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/client_or_server"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/netconf"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/renderer"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/world"
)

var (
	backgroundImage renderer.Image
)

type App struct {
	renderer.App

	hasInitialized bool
	clientOrServer netcode.Controller
	world          world.World
}

func (app *App) Init() {
	// Load assets
	{
		ent.LoadPlayerAssets(app.App)
		img, _, err := image.Decode(strings.NewReader(asset.Background))
		if err != nil {
			panic(err)
		}
		backgroundImage = app.NewImageFromImage(img)
	}

	app.SetRunnableOnUnfocused(true)

	// Load client/server
	app.clientOrServer = client_or_server.NewClientOrServer(netconf.Options{
		// note(jae): 2021-04-17
		// if hosting non-locally, this should be your servers remote IP
		// ie. PublicIP: "220.240.114.91",
		PublicIP: "127.0.0.1",
	})
}

func (app *App) Update() error {
	if !app.hasInitialized {
		app.Init()
		app.hasInitialized = true
	}

	//startTime := monotime.Now()

	// Update inputs
	app.world.PlayerInput = GetPlayerInput()
	for i := range app.world.Players {
		entity := app.world.Players[i]
		if entity != app.world.MyPlayer {
			continue
		}
		entity.Inputs = app.world.PlayerInput
	}

	// Handle networking
	// - client: create/update players
	// - server: take world snapshots, create/update players
	//clientOrServerStartTime := time.Now()
	app.clientOrServer.BeforeUpdate(&app.world)
	//fmt.Printf("client/server beforeUpdate time taken: %v\n", time.Since(clientOrServerStartTime))
	if !app.clientOrServer.HasStartedOrConnected() {
		return nil
	}

	// Simulate the game
	app.world.Update()

	//fmt.Printf("frame Time taken: %v\n", time.Duration(monotime.Now()-startTime))

	return nil
}

func (app *App) Draw(screen renderer.Screen) {
	if !app.clientOrServer.HasStartedOrConnected() {
		return
	}

	// Draws Background Image
	screen.DrawImage(backgroundImage, renderer.ImageOptions{
		ScaleX: 1.5,
		ScaleY: 1.5,
	})

	// Draw the players
	for _, entity := range app.world.Players {
		entity.Draw(screen)
	}

	// Show the message
	//msg := fmt.Sprintf("TPS: %0.2f\nPress the space key to jump.", ebiten.CurrentTPS())
	//ebitenutil.DebugPrint(screen, msg)
}

func GetPlayerInput() ent.PlayerInput {
	// Handle keyboard input
	playerInput := ent.PlayerInput{
		IsHoldingLeft:  input.IsKeyPressed(input.KeyA) || input.IsKeyPressed(input.KeyLeft),
		IsHoldingRight: input.IsKeyPressed(input.KeyD) || input.IsKeyPressed(input.KeyRight),
		IsHoldingJump:  input.IsKeyPressed(input.KeySpace),
	}

	// Detect mouse / touch for movement
	{
		windowWidth := world.ScreenWidth
		windowWidthThird := windowWidth / 3

		// Handle mouse
		if input.IsMouseButtonPressed(input.MouseButtonLeft) {
			x, _ := input.MousePosition()
			if x < windowWidthThird && !playerInput.IsHoldingRight {
				// Move left if touching left side of screen
				playerInput.IsHoldingLeft = true
			}
			if x > windowWidthThird && x < windowWidthThird*2 {
				// Jump if touching middle of screen
				playerInput.IsHoldingJump = true
			}
			if x > windowWidthThird*2 && !playerInput.IsHoldingLeft {
				// Move right if touching right side of screen
				playerInput.IsHoldingRight = true
			}
		}

		// Handle touch (mobile)
		// (TouchIDs returns nil for desktops)
		for _, touchID := range input.TouchIDs() {
			x, y := input.TouchPosition(touchID)
			if x == 0 && y == 0 {
				// skip if not touching anything
				continue
			}
			if x < windowWidthThird && !playerInput.IsHoldingRight {
				// Move left if touching left side of screen
				playerInput.IsHoldingLeft = true
			}
			if x > windowWidthThird && x < windowWidthThird*2 {
				// Jump if touching middle of screen
				playerInput.IsHoldingJump = true
			}
			if x > windowWidthThird*2 && !playerInput.IsHoldingLeft {
				// Move right if touching right side of screen
				playerInput.IsHoldingRight = true
			}
		}
	}

	return playerInput
}

func (g *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	return world.ScreenWidth, world.ScreenHeight
}

func StartApp() {
	app := &App{}
	app.SetWindowSize(world.ScreenWidth, world.ScreenHeight)
	app.SetWindowTitle("Toy MMO Platformer WebRTC")
	if err := app.App.RunGame(app); err != nil {
		panic(err)
	}
}
