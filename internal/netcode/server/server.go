package server

import (
	"bytes"
	"fmt"
	"io"
	"log"

	"github.com/silbinarywolf/toy-webrtc-mmo/internal/ent"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/netconf"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/netconst"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/packs"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/rtt"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netdriver/webrtcdriver/webrtcserver"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/world"
)

const (
	enableDebugPrintingInputFrameBuffer = false
)

// compile-time assert we implement this interface
var _ netcode.Controller = new(Controller)

func New(options netconf.Options) *Controller {
	net := &Controller{}
	net.server = webrtcserver.New(webrtcserver.Options{
		PublicIP:      options.PublicIP,
		ICEServerURLs: []string{"stun:" + options.PublicIP + ":3478"},
	})
	return net
}

type Controller struct {
	server          *webrtcserver.Server
	gameConnections []*gameConnection

	buf        *bytes.Buffer
	backingBuf [65536]byte

	hasStarted     bool
	worldSnapshots [][]byte
}

// gameConnection is data specifically related to game-logic and de-coupled from our network driver
type gameConnection struct {
	ID        uint16
	Player    *ent.Player
	IsUsed    bool
	AckPacket packs.AckPacket

	rtt         rtt.RoundTripTracking
	InputBuffer []packs.ClientFrameInput

	// LastInputFrameSimulated is the last frame number we've simulated
	LastInputFrameSimulated uint16
	// NextInputFrameToBeSimulated is the frame number we erecieved from the client that we're going to
	// simulate this frame
	NextInputFrameToBeSimulated uint16
}

func (net *Controller) init(world *world.World) {
	net.gameConnections = make([]*gameConnection, len(net.server.Connections()))
	for i := 0; i < len(net.server.Connections()); i++ {
		gameConn := &gameConnection{}
		// note: ID should never be 0
		gameConn.ID = uint16(i) + 1
		net.gameConnections[i] = gameConn
	}
	net.buf = bytes.NewBuffer(net.backingBuf[:])

	log.Printf("starting server...")
	net.server.Start()
	log.Printf("server started")
}

func (net *Controller) HasStartedOrConnected() bool {
	return net.server.IsListening()
}

func (net *Controller) BeforeUpdate(world *world.World) {
	if !net.hasStarted {
		net.init(world)
		net.hasStarted = true
	}

	// Take a snapshot of world state so we can rewind the universe and
	// playback a players actions when we receive inputs
	/* const maxSnapshotCount = 10
	if len(net.worldSnapshots) >= maxSnapshotCount {
		for i := 1; i < len(net.worldSnapshots); i++ {
			net.worldSnapshots[i-1] = net.worldSnapshots[i]
		}
		net.worldSnapshots[len(net.worldSnapshots)-1] = world.Snapshot()
	} else {
		net.worldSnapshots = append(net.worldSnapshots, world.Snapshot())
	} */

	for i, conn := range net.server.Connections() {
		gameConn := net.gameConnections[i]
		if !conn.IsConnected() {
			if gameConn.IsUsed {
				world.RemovePlayer(gameConn.Player)

				// Reset slot
				id := gameConn.ID
				*gameConn = gameConnection{}
				// note: ID should never be 0
				gameConn.ID = id
				conn.Free()
			}
			continue
		}
		if !gameConn.IsUsed {
			log.Printf("New connection! Creating new player...\n")
			gameConn.Player = world.CreatePlayer()
			gameConn.Player.NetID = gameConn.ID
			if gameConn.ID == 0 {
				panic("developer mistake, ID should never be 0")
			}
			gameConn.IsUsed = true
		}

		// read packets
	MainReadLoop:
		for {
			byteData, ok := conn.Read()
			if !ok {
				break
			}
			var buf bytes.Reader
			buf.Reset(byteData)
			for {
				sequenceID, packet, err := packs.Read(&buf)
				if err != nil {
					if err == io.EOF {
						break
					}
					log.Printf("unable to read packet: %v", err)
					continue
				}
				if _, ok := packet.(*packs.AckPacket); !ok {
					// to avoid recursion, we don't acknowledge acknowledgement packets
					gameConn.AckPacket.SequenceIDList = append(gameConn.AckPacket.SequenceIDList, sequenceID)
				}
				switch packet := packet.(type) {
				case *packs.AckPacket:
					for _, seqID := range packet.SequenceIDList {
						gameConn.rtt.Ack(seqID)
					}
				case *packs.ClientPlayerPacket:
					if len(packet.InputBuffer) > netconst.MaxServerInputBuffer {
						fmt.Printf("disconnecting client, they sent %d input packets when the limit is %d", len(packet.InputBuffer), netconst.MaxServerInputBuffer)
						conn.CloseButDontFree()
						break MainReadLoop
					}
					if len(packet.InputBuffer) == 0 {
						// do nothing
					} else {
						if len(gameConn.InputBuffer) > 0 {
							// We only use the given input buffer if the last item
							// is on a later frame than the current input buffer
							prevInputBuffer := gameConn.InputBuffer[len(gameConn.InputBuffer)-1]
							nextInputBuffer := packet.InputBuffer[len(packet.InputBuffer)-1]
							if rtt.IsWrappedUInt16GreaterThan(nextInputBuffer.Frame, prevInputBuffer.Frame) {
								gameConn.InputBuffer = packet.InputBuffer
							}
						} else {
							gameConn.InputBuffer = packet.InputBuffer
						}
					}
				default:
					log.Printf("unhandled packet type: %T", packet)
				}
			}
		}
	}

	// note(jae): 2021-04-03
	for i, conn := range net.server.Connections() {
		if !conn.IsConnected() {
			continue
		}
		gameConn := net.gameConnections[i]
		if !gameConn.IsUsed ||
			gameConn.Player == nil {
			// Skip if not used or have no player
			continue
		}
		// Get the next un-simulated input from the clients buffer of inputs
		// and set the client entity to use that when simulating this frame
		var inputBuffer packs.ClientFrameInput
		var foundCount int

		// note(jae): 2021-04-05
		// Consider replacing this with generous world rewinding... ie.
		// - get rtt of player (ie. 60ms - 300ms)
		// - rewind world N frames and process inputs in the past
		//
		// Why do this?
		// Well imagine you're playing a precise platformer like Celeste and you
		// press to jump to a platform and manage to just land on it by 1-pixel / 1 frame.
		// If the server ends up processing your input even 1 frame later, the server will end up
		// making you jump a frame later than you intended, and you'll just miss the platform.
		// ie. from the servers perspective, you pressed jump 1 frame late, so technically you never jumped and
		//     you just fell off.
		//
		// (Imagine you're lagging by just 30ms, this means the server will process your inputs 1-2 frames later.
		// 60ms? 3-4 frames. 350ms? 21-22 frames.)
		//
		// By rewinding the world to be what it was from the players perspective, they will definitely make the jump on the server-side.
		// *But* this means that if we decide to mix pixel precise platforming and VS battles, we can get inconsistencies
		// where players might see another player falling over for a bit, then be corrected suddenly... or hackers could potentially
		// create tools to just say "we jumped 10 frames in the past" if they miss a jump and the server will correct it.
		//
		// People on good network connections could potentially figure out how to abuse this in other creative ways without hacking
		// too.
		if enableDebugPrintingInputFrameBuffer {
			fmt.Printf("------ Frame --------\n")
			fmt.Printf("Last simulated: %d\n", gameConn.LastInputFrameSimulated)
		}
		for _, otherInputBuffer := range gameConn.InputBuffer {
			if otherInputBuffer.Frame == 0 {
				log.Printf("invalid packet data or developer mistake, frame should never be 0. closing")
				conn.CloseButDontFree()
				break
			}
			if rtt.IsWrappedUInt16GreaterThan(otherInputBuffer.Frame, gameConn.LastInputFrameSimulated) {
				if foundCount == 0 {
					if enableDebugPrintingInputFrameBuffer {
						fmt.Printf("\n- Frame (found): %d\n", otherInputBuffer.Frame)
					}
					inputBuffer = otherInputBuffer
				} else if enableDebugPrintingInputFrameBuffer {
					fmt.Printf("%d, ", otherInputBuffer.Frame)
				}
				foundCount++
				continue
			}
			if enableDebugPrintingInputFrameBuffer {
				fmt.Printf("%d, ", otherInputBuffer.Frame)
			}
		}
		if enableDebugPrintingInputFrameBuffer {
			fmt.Printf("\n------ End Frame --------\n")
			fmt.Printf("Net RTT: %v\n", gameConn.rtt.Latency())
		}
		// note(jae): 2021-04-05
		// we might want to adjust this later so we can handle
		// input more smoothly with jitter
		// ie. foundCount > 1 or foundCount > 2
		//
		// We want to smooth out input packets because they can sometimes arrive inconsistently
		// ie. frame 1 - get 1 input packet
		//	   frame 2 - no input packet
		//	   frame 3 - get 2 input packets
		//
		// Because packets don't necessarily arrive together, a jitter buffer ensures that groups of inputs
		// are processed together such as a combo in a fighting game, or a precise platforming manuver.
		// Without this, a gap in the inputs being processed could lead to a combo/manuver being broken.
		//
		// The problem with using an input jitter buffer though is that we end up processing frames later.
		// (Which might be a non-problem if we implement the rewinding world system mentioned above in a big comment)
		if foundCount > 0 {
			if inputBuffer.Frame == 0 {
				log.Printf("invalid packet data or developer mistake, frame should never be 0. closing")
				conn.CloseButDontFree()
				break
			}
			gameConn.Player.Inputs = inputBuffer.PlayerInput
			gameConn.LastInputFrameSimulated = gameConn.NextInputFrameToBeSimulated
			gameConn.NextInputFrameToBeSimulated = inputBuffer.Frame
		} else {
			// reset to all zero values
			gameConn.Player.Inputs = ent.PlayerInput{}
		}
	}

	// Send player data to everybody on every frame
	// (this is not good engineering, this isnt even OK engineering)
	for i, conn := range net.server.Connections() {
		if !conn.IsConnected() {
			continue
		}
		gameConn := net.gameConnections[i]
		if !gameConn.IsUsed {
			// skip if not used
			continue
		}
		net.buf.Reset()
		if len(gameConn.AckPacket.SequenceIDList) > 0 {
			if err := packs.Write(net.buf, &gameConn.rtt, &gameConn.AckPacket); err != nil {
				log.Printf("failed to write ack packet: %v, closing connection", err)
				conn.CloseButDontFree()
				continue
			}
			gameConn.AckPacket.SequenceIDList = gameConn.AckPacket.SequenceIDList[:0]
		}
		if player := gameConn.Player; player != nil {
			stateUpdateList := make([]packs.PlayerState, 0, len(world.Players))
			for _, entity := range world.Players {
				stateUpdateList = append(stateUpdateList, packs.PlayerState{
					NetID:   entity.NetID,
					X:       entity.X,
					Y:       entity.Y,
					Hspeed:  entity.Hspeed,
					Vspeed:  entity.Vspeed,
					DirLeft: entity.DirLeft,
				})
			}
			if err := packs.Write(net.buf, &gameConn.rtt, &packs.ServerWorldStatePacket{
				MyNetID:                 player.NetID,
				LastSimulatedInputFrame: gameConn.LastInputFrameSimulated,
				Players:                 stateUpdateList,
			}); err != nil {
				log.Printf("failed to write world update packet: %v", err)
				conn.CloseButDontFree()
				continue
			}
		}
		// Upper limit of packets in gamedev are generally: "something like 1000 to 1200 bytes of payload data"
		// source: https://www.gafferongames.com/post/packet_fragmentation_and_reassembly/
		if net.buf.Len() > 1000 {
			// note(jae): 2021-04-02
			// when i looked at raw packet data in Wireshark, packets were about ~100 bytes, even if i was sending ~20 bytes
			// of data. DTLS v1.2 / WebRTC / DataChannels may have a 100 byte overhead that I need to consider
			// when printing this kind of warning logic
			log.Printf("warning: size of packet is %d, should be conservative and fit between 1000-1200", net.buf.Len())
		}
		// DEBUG: uncomment to debug packet size
		//log.Printf("note: size of packet is %d (rtt latency: %v)", net.buf.Len(), gameConn.rtt.Latency())

		if err := conn.Send(net.buf.Bytes()); err != nil {
			log.Printf("failed to send: %v", err)
			conn.CloseButDontFree()
			continue
		}
	}
}
