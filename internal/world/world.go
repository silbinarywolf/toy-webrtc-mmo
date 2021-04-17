package world

import (
	"bytes"

	"github.com/silbinarywolf/toy-webrtc-mmo/internal/ent"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/packbuf"
)

const (
	ScreenWidth  = 1280
	ScreenHeight = 720
)

// worldSnapshot contains all data we want to be stored per-frame
// for rewinding/replaying the simulation
type worldSnapshot struct {
	Players []*ent.Player
}

type World struct {
	worldSnapshot
	PlayerInput ent.PlayerInput
	MyPlayer    *ent.Player
}

func SyncWorldToSnapshot(worldSnapshot *worldSnapshot, snapshotData []byte) {
	r := bytes.NewReader(snapshotData)
	if err := packbuf.Read(r, worldSnapshot); err != nil {
		panic(err)
	}
}

func (worldSnapshot *worldSnapshot) Snapshot() []byte {
	w := bytes.NewBuffer(nil)
	if err := packbuf.Write(w, worldSnapshot); err != nil {
		panic(err)
	}
	return w.Bytes()
}

func (world *World) CreatePlayer() *ent.Player {
	// Add initial player (just for testing for now)
	world.Players = append(world.Players, &ent.Player{
		X: 180,
		Y: 180,
	})
	entity := world.Players[len(world.Players)-1]
	entity.Init()
	return entity
}

func (world *World) RemovePlayer(player *ent.Player) {
	// note(jae): 2021-03-04
	// slow-ordered remove. reasoning right now is because we want
	// draw order to be consistent and im being real lazy.
	for i, otherPlayer := range world.Players {
		if player == otherPlayer {
			world.Players = append(world.Players[:i], world.Players[i+1:]...)
			return
		}
	}
}

func (world *World) Update() {
	for i := range world.Players {
		entity := world.Players[i]
		entity.Update()
	}
}
