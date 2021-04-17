package webrtcserver

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/pion/turn/v2"
	"github.com/pion/webrtc/v3"
	"github.com/pkg/errors"

	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netdriver/webrtcdriver/webrtcserver/stunserver"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netdriver/webrtcdriver/webrtcshared"
)

const (
	defaultHttpPort             = 50000
	defaultMaxConnections       = 256
	defaultPacketLimitPerClient = 256
)

type Server struct {
	api         *webrtc.API
	options     Options
	stunServer  *turn.Server
	connections []*Connection
}

type Options struct {
	// MaxConnections is the maximum client connections
	//
	// If not set, this will default to 256
	MaxConnections int
	// HttpPort of the SDP handler
	//
	// If not set, this will default to 50000
	HttpPort      int
	PublicIP      string
	ICEServerURLs []string

	isListening atomic.Value
}

type Connection struct {
	mu             sync.Mutex
	peerConnection *webrtc.PeerConnection
	dataChannel    *webrtc.DataChannel
	packets        chan []byte
	isConnected    bool
	isUsed         bool
}

func (s *Server) Connections() []*Connection {
	return s.connections
}

func (conn *Connection) IsConnected() bool {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.isConnected
}

func (conn *Connection) Read() ([]byte, bool) {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	select {
	case data := <-conn.packets:
		return data, true
	default:
		// if no data
		return nil, false
	}
}

func (conn *Connection) Send(data []byte) error {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if conn.dataChannel == nil {
		// if connection closed, we don't care that
		// the packet never sent
		//
		// using io.ErrClosedPipe for nil dataChannels as
		// that's what dataChannel.Send() will use if the
		// pipe has been closed
		return io.ErrClosedPipe
	}
	return conn.dataChannel.Send(data)
}

// CloseButDontFree will close down the connection
//
// But it won't free up the server slot, that should be handled in a loop at the start
// of the frame so it can cleanup player objects / etc
//
// ie.
// for i, conn := range net.server.Connections() {
// 		gameConn := net.gameConnections[i]
//		if !conn.IsConnected() && {
//			if gameConn.IsUsed {
//				world.RemovePlayer(gameConn.Player)
//				conn.Free()
//			}
//			continue
//		}
// }
func (conn *Connection) CloseButDontFree() {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	conn.needsMutexLock_disconnectButKeepMarkedAsUsed()
}

// needsMutexLock_disconnectButKeepMarkedAsUsed will close the connection but the connection
// slot will stay taken until the consuming code calls the "Free" method
//
// As the prefix suggests, you need to lock the conn and unlock before/after calling this
func (conn *Connection) needsMutexLock_disconnectButKeepMarkedAsUsed() {
	if conn.peerConnection != nil {
		conn.peerConnection.Close()
		conn.peerConnection = nil
	}
	if conn.dataChannel != nil {
		conn.dataChannel.Close()
		conn.dataChannel = nil
	}
	conn.packets = nil
	conn.isConnected = false

	// note(jae): 2021-04-04
	// isUsed must stay as-is, we only allow this connection slot to be reused
	// after the consuming code of this library calls "Free()" on the connection
}

// Free must be called after a clients disconnection in consumer / user-code.
func (conn *Connection) Free() {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if conn.isConnected {
		panic("cannot call Free if connection is still connected")
	}
	conn.isUsed = false
}

func New(options Options) *Server {
	if options.HttpPort == 0 {
		options.HttpPort = defaultHttpPort
	}
	if options.MaxConnections == 0 {
		options.MaxConnections = defaultMaxConnections
	}
	if options.PublicIP == "" {
		panic("cannot provide empty IP address")
	}

	s := &Server{}
	s.options.isListening.Store(false)
	s.options = options
	s.connections = make([]*Connection, options.MaxConnections)
	for i := 0; i < options.MaxConnections; i++ {
		conn := &Connection{}
		s.connections[i] = conn
	}
	return s
}

func (s *Server) IsListening() bool {
	v, ok := s.options.isListening.Load().(bool)
	if !ok {
		return false
	}
	return v
}

func (s *Server) handleSDP(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		http.Error(w, "Please send a request body", 400)
		return
	}
	// TODO(jae): 2021-02-28
	// Need to think about origin headers and consider locking this down
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", http.MethodPost)
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Please send a "+http.MethodPost+" request", 400)
		return
	}

	var offer webrtc.SessionDescription
	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		message := "error decoding offer"
		log.Printf("%s: %v", message, err)
		http.Error(w, message, 500)
		return
	}

	// Prepare the configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				// URLs of ICE servers (can be STUN or TURN but we just use STUN)
				// eg. []string{"stun:stun.l.google.com:19302"}
				URLs: s.options.ICEServerURLs,
			},
		},
		SDPSemantics: webrtc.SDPSemanticsUnifiedPlan,
	}

	// Create a new RTCPeerConnection
	// TODO(Jae): 2020-05-05
	// Figure out how to either manually create a DataChannel
	// or set it up so its using UDP.
	// - maxRetransmits: 0
	// - ordered: 0
	// Source: https://www.html5rocks.com/en/tutorials/webrtc/datachannels/
	peerConnection, err := s.api.NewPeerConnection(config)
	if err != nil {
		message := "error creating peer connection"
		log.Printf("%s: %v", message, err)
		http.Error(w, message, 500)
		return
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		switch connectionState {
		case webrtc.ICEConnectionStateClosed,
			webrtc.ICEConnectionStateFailed:
			peerConnection.Close()
			// note(jae): 2021-04-15
			// explicitly don't handle "disconnected" state as this can be temporary
			// and change back to "connected" state in flaky networks
			// see: https://developer.mozilla.org/en-US/docs/Web/API/RTCPeerConnection/iceConnectionState
			//case webrtc.ICETransportStateDisconnected:
		}
	})

	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		peerConnection.Close()
		message := "error creating setting remote description"
		log.Printf("%s: %v", message, err)
		http.Error(w, message, 500)
		return
	}

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		peerConnection.Close()
		message := "error creating creating answer"
		log.Printf("%s: %v", message, err)
		http.Error(w, message, 500)
		return
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		peerConnection.Close()
		message := "error creating creating answer"
		log.Printf("%s: %v", message, err)
		http.Error(w, message, 500)
		return
	}

	waitChannel := make(chan struct{})

	candidateList := make([]webrtc.ICECandidateInit, 0)
	candidateErrors := make([]error, 0)
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			// if nil than all ICE candidates have been sent from the client
			close(waitChannel)
			return
		}
		candidateInit := candidate.ToJSON()
		if err := peerConnection.AddICECandidate(candidateInit); err != nil {
			// note(jae): 2021-04-15
			// We dont do atomics/channels for sending back "hasError" as we assume
			// only this goroutine will be writing/reading to "hasError" at this point in time.
			// I doubt how sound this is but I'm not sure how to test this in anger either.
			candidateErrors = append(candidateErrors, err)
			return
		}
		// note(jae): 2021-04-15
		// We dont do atomics/channels for sending back candidates as we assume
		// only this goroutine will be writing/reading to candidates at this point in time.
		// I doubt how sound this is but I'm not sure how to test this in anger either.
		candidateList = append(candidateList, candidateInit)
	})

	<-waitChannel

	if len(candidateErrors) > 0 {
		peerConnection.Close()
		message := "unexpected error. unable to add ice candidate(s)"
		log.Printf("%s: %v", message, candidateErrors)
		http.Error(w, message, 500)
		return
	}
	if len(candidateList) == 0 {
		peerConnection.Close()
		message := "unexpected error. received 0 candidates"
		log.Printf("%s: %v", message, err)
		http.Error(w, message, 500)
		return
	}

	// Find a free connection slot
	var foundConn *Connection
	for _, conn := range s.connections {
		conn.mu.Lock()
		if conn.isUsed {
			conn.mu.Unlock()
			continue
		}
		conn.isUsed = true
		conn.peerConnection = peerConnection
		conn.mu.Unlock()

		foundConn = conn
		break
	}
	if conn := foundConn; conn == nil {
		peerConnection.Close()

		message := "server is full"
		log.Print(message)
		http.Error(w, message, 503)
		return
	}

	peerConnection.OnDataChannel(foundConn.onDataChannel)

	if err := json.NewEncoder(w).Encode(&webrtcshared.ConnectResponse{
		Candidates: candidateList,
		Answer:     answer,
	}); err != nil {
		foundConn.mu.Lock()
		foundConn.needsMutexLock_disconnectButKeepMarkedAsUsed()
		foundConn.isUsed = false
		foundConn.mu.Unlock()

		message := "unexpected error, unable to encode connection response"
		log.Printf("%s: %v", message, err)
		http.Error(w, message, 500)
		return
	}
}

func (conn *Connection) onDataChannel(dataChannel *webrtc.DataChannel) {
	if err := isValidDataChannel(dataChannel); err != nil {
		log.Printf("invalid data channel: %v", err)

		// close incoming datachannel
		dataChannel.Close()

		// close connection
		// - if we don't have a single data channel yet
		// - if we got another data channel after the first (should never happen)
		conn.mu.Lock()
		conn.needsMutexLock_disconnectButKeepMarkedAsUsed()
		conn.isUsed = false
		conn.mu.Unlock()
		return
	}

	// setup connection
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if conn.dataChannel != nil {
		// if we already have a data channel, close the new one
		// and ignore it
		//
		// In a real world scenario we might want to consider closing the connection
		// completely and logging it, as this case occurring could be considered a hack
		// attempt.
		//
		// (ie. someone making the app open another data channel when it was
		// never designed to)
		dataChannel.Close()
		return
	}
	conn.packets = make(chan []byte)
	// note(jae): 2021-04-04
	// we only consider a client actually connected once a datachannel
	// is opened. This is because if there are UDP port forwarding issues on the server
	// this codepath will never be reached.
	conn.isConnected = true
	conn.dataChannel = dataChannel
	conn.dataChannel.OnMessage(conn.onDataChannelMessage)
	conn.dataChannel.OnClose(conn.onDataChannelClose)
}

func isValidDataChannel(dataChannel *webrtc.DataChannel) error {
	if dataChannel.Ordered() {
		return errors.New("DataChannel tried to connect with \"ordered: true\". Server accepts \"ordered: false\" only for UDP")
	}
	if dataChannel.MaxRetransmits() == nil {
		return errors.New("DataChannel tried to connect with \"maxRetransmits\" not equal to 0. Was nil. Must be 0 for UDP")
	}
	if maxRetransmits := *dataChannel.MaxRetransmits(); maxRetransmits != 0 {
		return errors.New("DataChannel tried to connect with \"maxRetransmits\" not equal to 0. Instead was %v. Must be 0 for UDP")
	}
	maxPacketLifeTime := dataChannel.MaxPacketLifeTime()
	if maxPacketLifeTime != nil {
		return errors.New("DataChannel tried to connect with \"maxPacketLifeTime\" not nil. Must be nil for UDP")
	}
	return nil
}

func (conn *Connection) onDataChannelClose() {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	conn.needsMutexLock_disconnectButKeepMarkedAsUsed()
}

func (conn *Connection) onDataChannelMessage(msg webrtc.DataChannelMessage) {
	conn.packets <- msg.Data
}

func (s *Server) Start() {
	go func() {
		if err := s.start(); err != nil {
			panic(err)
		}
	}()
}

func (s *Server) start() error {
	s.options.isListening.Store(false)
	stunServer, err := stunserver.ListenAndStart(s.options.PublicIP)
	if err != nil {
		return errors.Wrap(err, "failed to start stun server")
	}
	s.stunServer = stunServer
	defer s.stunServer.Close()

	// Setup WebRTC settings
	settings := webrtc.SettingEngine{}

	// note(jae): 2021-03-27
	// Set explicit UDP port ranges to allow on server box
	if err := settings.SetEphemeralUDPPortRange(10000, 11999); err != nil {
		return errors.Wrap(err, "failed to set UDP port range for server")
	}
	s.api = webrtc.NewAPI(webrtc.WithSettingEngine(settings))

	http.HandleFunc("/sdp", s.handleSDP)

	httpServer := &http.Server{
		Addr:    ":" + strconv.Itoa(s.options.HttpPort),
		Handler: nil,
	}
	ln, err := net.Listen("tcp", httpServer.Addr)
	if err != nil {
		return errors.Wrap(err, "failed to listen on "+httpServer.Addr)
	}
	s.options.isListening.Store(true)
	if err := httpServer.Serve(ln); err != nil {
		return errors.Wrap(err, "server closed")
	}
	return nil
}
