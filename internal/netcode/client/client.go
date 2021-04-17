package client

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/netconf"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/netconst"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/packs"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/rtt"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netdriver/webrtcdriver/webrtcclient"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/world"
)

// compile-time assert we implement this interface
var _ netcode.Controller = new(Controller)

func New(options netconf.Options) *Controller {
	net := &Controller{}
	net.client = webrtcclient.New(webrtcclient.Options{
		IPAddress:     options.PublicIP + ":50000",
		ICEServerURLs: []string{"stun:" + options.PublicIP + ":3478"},
	})
	return net
}

type Controller struct {
	client *webrtcclient.Client

	buf        *bytes.Buffer
	backingBuf [65536]byte
	rtt        rtt.RoundTripTracking
	ackPacket  packs.AckPacket

	frameCounter     uint16
	frameInputBuffer []packs.ClientFrameInput

	hasStarted   bool
	hasConnected bool
}

/* func (net *Controller) IsConnected() bool {
	return net.client.IsConnected()
} */

func (net *Controller) HasStartedOrConnected() bool {
	return net.client.IsConnected()
}

func (net *Controller) init() {
	net.buf = bytes.NewBuffer(net.backingBuf[:])
	net.client.Start()
}

func (net *Controller) BeforeUpdate(world *world.World) {
	if !net.hasStarted {
		net.init()
		net.hasStarted = true
	}
	if err := net.client.GetLastError(); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	if !net.client.IsConnected() {
		// If not ready yet, don't try to process packets
		return
	}
	if !net.hasConnected {
		// init
		world.MyPlayer = world.CreatePlayer()
		net.hasConnected = true
	}

	// Get frame count starting at 1
	net.frameCounter++
	if net.frameCounter == 0 {
		net.frameCounter++
	}

	// Store last N frames of inputs and remove from list as
	// server processes them
	if player := world.MyPlayer; player != nil {
		// maxClientInputBuffer is how many frames of input we hold onto so
		// that when we get world state from the server, we can replay our inputs
		// that haven't been simulated by the server yet.
		const maxClientInputBuffer = 60

		frameInputBufferData := packs.ClientFrameInput{
			Frame:       net.frameCounter,
			PlayerInput: player.Inputs,
		}
		if len(net.frameInputBuffer) >= maxClientInputBuffer {
			// move everything down 1 slot...
			for i := 1; i < len(net.frameInputBuffer); i++ {
				net.frameInputBuffer[i-1] = net.frameInputBuffer[i]
			}
			// override last record
			net.frameInputBuffer[len(net.frameInputBuffer)-1] = frameInputBufferData
		} else {
			net.frameInputBuffer = append(net.frameInputBuffer, frameInputBufferData)
		}
	}

	var lastWorldStatePacket *packs.ServerWorldStatePacket
	for {
		byteData, ok := net.client.Read()
		if !ok {
			// If no more packet data
			break
		}
		var buf bytes.Reader
		buf.Reset(byteData)
		for {
			sequenceID, packet, err := packs.Read(&buf)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				panic(err)
			}
			if _, ok := packet.(*packs.AckPacket); !ok {
				// To avoid recursion, we don't acknowledge acknowledgement packets
				net.ackPacket.SequenceIDList = append(net.ackPacket.SequenceIDList, sequenceID)
			}
			switch packet := packet.(type) {
			case *packs.AckPacket:
				for _, seqID := range packet.SequenceIDList {
					net.rtt.Ack(seqID)
				}
			case *packs.ServerWorldStatePacket:
				if packet.MyNetID == 0 {
					log.Printf("Bad packet from server, has ID of 0: %+v", packet)
					continue
				}
				// We only want to use the latest up to date world state
				if lastWorldStatePacket != nil {
					if rtt.IsWrappedUInt16GreaterThan(packet.LastSimulatedInputFrame, lastWorldStatePacket.LastSimulatedInputFrame) {
						lastWorldStatePacket = packet
					}
				} else {
					lastWorldStatePacket = packet
				}
			default:
				panic(fmt.Sprintf("unhandled packet type: %T", packet))
			}
		}
	}

	// Use the latest up-to-date world state and snap to it
	// (also replay inputs the server hasn't simulated yet)
	if packet := lastWorldStatePacket; packet != nil {
		if world.MyPlayer != nil {
			world.MyPlayer.NetID = packet.MyNetID
		}
		for _, state := range packet.Players {
			hasFound := false
			for _, entity := range world.Players {
				if entity.NetID != state.NetID {
					continue
				}
				entity.X = state.X
				entity.Y = state.Y
				entity.Hspeed = state.Hspeed
				entity.Vspeed = state.Vspeed
				entity.DirLeft = state.DirLeft
				hasFound = true
			}
			if !hasFound {
				entity := world.CreatePlayer()
				entity.NetID = state.NetID
				entity.X = state.X
				entity.Y = state.Y
				entity.Hspeed = state.Hspeed
				entity.Vspeed = state.Vspeed
				entity.DirLeft = state.DirLeft
			}
		}

		// Replay inputs that haven't been processed by the server yet
		{
			var unprocessedFrameInputBuffer []packs.ClientFrameInput
			unprocessedCount := 0
			for i, frameInput := range net.frameInputBuffer {
				if rtt.IsWrappedUInt16GreaterThan(frameInput.Frame, packet.LastSimulatedInputFrame) {
					if unprocessedCount == 0 {
						// Only keep first item in frame input buffer
						unprocessedFrameInputBuffer = net.frameInputBuffer[i:]
					}
					// unprocessedCount is staying around for mostly future debugging purposes
					// this could be swapped to a boolean
					unprocessedCount++
				}
			}
			if unprocessedCount > 0 {
				if player := world.MyPlayer; player != nil {
					prevInput := player.Inputs
					for _, inputFrame := range unprocessedFrameInputBuffer {
						player.Inputs = inputFrame.PlayerInput
						player.Update()
					}
					player.Inputs = prevInput
				}
			}
		}
	}

	// Send player input and acks to server every frame
	{
		net.buf.Reset()
		if len(net.ackPacket.SequenceIDList) > 0 {
			if err := packs.Write(net.buf, &net.rtt, &net.ackPacket); err != nil {
				panic(err)
			}
			net.ackPacket.SequenceIDList = net.ackPacket.SequenceIDList[:0]
		}
		{
			frameInputBuffer := net.frameInputBuffer
			if len(frameInputBuffer) > netconst.MaxServerInputBuffer {
				frameInputBuffer = frameInputBuffer[len(frameInputBuffer)-netconst.MaxServerInputBuffer:]
			}
			if err := packs.Write(net.buf, &net.rtt, &packs.ClientPlayerPacket{
				InputBuffer: frameInputBuffer,
			}); err != nil {
				panic(err)
			}
		}
		// Upper limit of packets in gamedev are generally: "something like 1000 to 1200 bytes of payload data"
		// source: https://www.gafferongames.com/post/packet_fragmentation_and_reassembly/
		if net.buf.Len() > 1000 {
			// note(jae): 2021-04-02
			// when i looked at raw packet data in Wireshark, packets were about ~100 bytes, even if i was sending ~20 bytes
			// of data. DTLS v1.2 / WebRTC / DataChannels may have a 100 byte overhead that I need to consider
			// when printing this kind of warning logic
			log.Printf("warning: client size of packet is %d, should be conservative and fit between 1000-1200", net.buf.Len())
		}
		// DEBUG: uncomment to debug packet size
		//log.Printf("note: client sending size of packet is %d (rtt latency: %v)", net.buf.Len(), net.rtt.Latency())
		if err := net.client.Send(net.buf.Bytes()); err != nil {
			panic(err)
		}
	}

	//fmt.Printf("Net RTT: %v\n", net.rtt.Latency())
}
