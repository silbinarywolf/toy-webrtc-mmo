// packs is a package that holds the packet data structures and associates an ID with them
package packs

import (
	"encoding/binary"
	"io"
	"reflect"
	"strconv"

	"github.com/silbinarywolf/toy-webrtc-mmo/internal/ent"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/packbuf"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/rtt"
)

const (
	// packetInvalid            PacketID = 0
	packetAck                PacketID = 1
	packetClientPlayerUpdate PacketID = 2
	packetWorldStateUpdate   PacketID = 3
)

// AckPacket is used by client/server to acknowledge that it recieved
// a packet (or multiple packets)
type AckPacket struct {
	SequenceIDList []uint16
}

func (packet *AckPacket) ID() PacketID {
	return packetAck
}

func init() {
	register(&AckPacket{})
}

// ClientPlayerPacket is the data sent from the client to the server
type ClientPlayerPacket struct {
	InputBuffer []ClientFrameInput
}

func (packet *ClientPlayerPacket) ID() PacketID {
	return packetClientPlayerUpdate
}

type ClientFrameInput struct {
	Frame uint16
	ent.PlayerInput
}

func init() {
	register(&ClientPlayerPacket{})
}

// ServerWorldStatePacket is the data sent from the server to the client
type ServerWorldStatePacket struct {
	MyNetID uint16
	// LastSimulatedInputFrame is the last input the server simulated in world.Update
	// this is utilized by the client to replay inputs that haven't been processed
	// by the server yet
	LastSimulatedInputFrame uint16
	Players                 []PlayerState
}

// PlayerState is the player state information we want to send per frame
//
// TODO(jae): 2021-04-02
// lets add a system like packbuf package where we just pass in a slice
// of []*ent.Player and any fields marked with `net:"x"` etc, will get sent
// over the wire
type PlayerState struct {
	NetID          uint16
	X, Y           float32
	Hspeed, Vspeed float32
	DirLeft        bool
}

func (packet *ServerWorldStatePacket) ID() PacketID {
	return packetWorldStateUpdate
}

func init() {
	register(&ServerWorldStatePacket{})
}

type PacketID uint8

var packetIDToType = make(map[PacketID]reflect.Type)

type Packet interface {
	ID() PacketID
}

type InvalidPacketID struct {
	id PacketID
}

func (err *InvalidPacketID) Error() string {
	return "invalid packet id: " + strconv.Itoa(int(err.id))
}

func register(packet Packet) {
	id := packet.ID()
	if _, ok := packetIDToType[id]; ok {
		panic("cannot register a packet with the same id twice: " + strconv.Itoa(int(id)))
	}
	packetIDToType[id] = reflect.TypeOf(packet).Elem()
}

func Read(r io.Reader) (uint16, Packet, error) {
	var packetID PacketID
	if err := binary.Read(r, binary.LittleEndian, &packetID); err != nil {
		return 0, nil, err
	}
	packetType, ok := packetIDToType[packetID]
	if !ok {
		return 0, nil, &InvalidPacketID{
			id: packetID,
		}
	}
	var seqID uint16
	if err := binary.Read(r, binary.LittleEndian, &seqID); err != nil {
		return 0, nil, err
	}
	packet := reflect.New(packetType).Interface().(Packet)
	if err := packbuf.Read(r, packet); err != nil {
		return 0, nil, err
	}
	return seqID, packet, nil
}

func Write(w io.Writer, rtt *rtt.RoundTripTracking, packet Packet) error {
	if err := binary.Write(w, binary.LittleEndian, packet.ID()); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, rtt.Next()); err != nil {
		return err
	}
	if err := packbuf.Write(w, packet); err != nil {
		return err
	}
	return nil
}
