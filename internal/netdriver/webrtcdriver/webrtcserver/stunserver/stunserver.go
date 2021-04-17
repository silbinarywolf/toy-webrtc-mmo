package stunserver

import (
	"fmt"
	"net"
	"strconv"

	"github.com/pion/stun"
	"github.com/pion/turn/v2"
	"github.com/pkg/errors"
)

const (
	port       = 3478
	hasLogging = true
)

// stunLogger wraps a PacketConn and prints incoming/outgoing STUN packets
// This pattern could be used to capture/inspect/modify data as well
type stunLogger struct {
	net.PacketConn
}

func (s *stunLogger) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	if n, err = s.PacketConn.WriteTo(p, addr); err == nil && stun.IsMessage(p) {
		msg := &stun.Message{Raw: p}
		if err = msg.Decode(); err != nil {
			return
		}

		fmt.Printf("Outbound STUN: %s \n", msg.String())
	}

	return
}

func (s *stunLogger) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	if n, addr, err = s.PacketConn.ReadFrom(p); err == nil && stun.IsMessage(p) {
		msg := &stun.Message{Raw: p}
		if err = msg.Decode(); err != nil {
			return
		}

		fmt.Printf("Inbound STUN: %s \n", msg.String())
	}

	return
}

// turnLogger wraps a Listener and prints accepting TURN connections
/* type turnLogger struct {
	net.Listener
}

// Accept waits for and returns the next connection to the listener.
func (t *turnLogger) Accept() (net.Conn, error) {
	conn, err := t.Listener.Accept()
	if err != nil {
		fmt.Printf("Failed TURN connection: %v", err)
		return nil, err
	}
	fmt.Printf("New TURN connection...")
	return conn, err
} */

func ListenAndStart(publicIP string) (*turn.Server, error) {
	if publicIP == "" {
		return nil, errors.New("cannot give empty string for public ip")
	}
	portStr := strconv.Itoa(port)
	publicIPParsed := net.ParseIP(publicIP)

	//tcpListener, err := net.Listen("tcp4", "0.0.0.0:"+portStr)
	//if err != nil {
	//	return nil, errors.Wrap(err, "failed to create TURN server listener")
	//}
	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:"+portStr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create STUN server listener")
	}
	relayAddressGenerator := &turn.RelayAddressGeneratorStatic{
		RelayAddress: publicIPParsed,
		Address:      "0.0.0.0",
	}
	if hasLogging {
		//tcpListener = &turnLogger{Listener: tcpListener}
		udpListener = &stunLogger{udpListener}
	}
	s, err := turn.NewServer(turn.ServerConfig{
		// Realm SHOULD be the domain name of the provider of the TURN server.
		// https://stackoverflow.com/a/63930426/5013410
		Realm: "silbinarywolf.com",
		// Set AuthHandler callback
		// This is called everytime a user tries to authenticate with the TURN server
		// Return the key for that user, or false when no user is found
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			// note(jae): 2021-04-15
			// this doesn't affect as we don't handle TURN for reasons commented below
			// CTRL+F/Search for: "low-latency games"
			// fmt.Printf("username: %s, realm: %s, srcAddr: %v\n", username, realm, srcAddr)
			return nil, false
		},
		// PacketConnConfigs is a list of UDP Listeners and the configuration around them
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn:            udpListener,
				RelayAddressGenerator: relayAddressGenerator,
			},
		},
		// note(Jae): 2021-03-14
		// This TURN stuff hasn't been properly tested and so if uncommented
		// I'm unsure if it even works.
		//
		// In anycase, I dont think we *want* a TURN server based on my reading of this:
		// https://bloggeek.me/google-free-turn-server/
		//
		// My current assumption is that we do NOT want it for low-latency games as it relays
		// all TCP packets through it. TCP is slow and not ideal for real-time game designs
		// like say Overwatch.
		//
		// ListenerConfigs: []turn.ListenerConfig{
		//	{
		//		Listener:              tcpListener,
		//		RelayAddressGenerator: relayAddressGenerator,
		//	},
		//},
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to start TURN/STUN server")
	}

	// note(jae): 2021-04-15
	// after start-up the server never really wants to close this down but i've kept
	// the code here as a note to self on how to close it if we needed to
	//if err = s.Close(); err != nil {
	//	return errors.Wrap(err, "unable to close TURN/STUN server")
	//}

	return s, nil
}
