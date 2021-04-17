package webrtcclient

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v3"
	"github.com/pkg/errors"
	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netdriver/webrtcdriver/webrtcshared"
)

type Client struct {
	options Options

	mu             sync.Mutex
	packets        chan []byte
	peerConnection *webrtc.PeerConnection
	dataChannel    *webrtc.DataChannel

	lastAtomicError atomic.Value

	// NOTE(Jae): 2020-06-05
	// It'd probably be a simpler/better API if instead
	// these were 1 value with 3 states:
	// - Connecting
	// - Connected
	// - Disconnected
	// But I can't be bothered rewriting this right now or figuring
	// out how to make that work with atomics correctly.
	_hasConnectedOnce atomic.Value
	_isConnected      atomic.Value
}

type Options struct {
	IPAddress     string
	ICEServerURLs []string
}

func New(options Options) *Client {
	if options.IPAddress == "" {
		panic("cannot provide empty IP address")
	}
	client := &Client{}
	client.options = options
	client._hasConnectedOnce.Store(false)
	client.setIsConnected(false)
	return client
}

func (client *Client) setIsConnected(v bool) {
	client._isConnected.Store(v)
}

func (client *Client) IsConnected() bool {
	v := client._isConnected.Load().(bool)
	return v
}

func (client *Client) setHasConnectedOnce(v bool) {
	client._hasConnectedOnce.Store(v)
}

func (client *Client) HasConnectedOnce() bool {
	v := client._hasConnectedOnce.Load().(bool)
	return v
}

func (client *Client) Disconnect() {
	client.close()
	client.setIsConnected(false)
}

func (client *Client) close() {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.dataChannel != nil {
		client.dataChannel.Close()
		client.dataChannel = nil
	}
	if client.peerConnection != nil {
		client.peerConnection.Close()
		client.peerConnection = nil
	}
}

func (client *Client) GetLastError() error {
	v := client.lastAtomicError.Load()
	if v == nil {
		return nil
	}
	return v.(error)
}

func (client *Client) Start() {
	go func() {
		if err := client.start(); err != nil {
			client.lastAtomicError.Store(err)
			return
		}
	}()
}

func (client *Client) Read() ([]byte, bool) {
	client.mu.Lock()
	defer client.mu.Unlock()
	select {
	case data := <-client.packets:
		return data, true
	default:
		// if no data
		return nil, false
	}
}

func (client *Client) Send(data []byte) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.dataChannel == nil {
		return nil
	}
	err := client.dataChannel.Send(data)
	return err
}

func postConnect(ipAddress string, offer webrtc.SessionDescription) (webrtcshared.ConnectResponse, error) {
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(offer)
	if err != nil {
		return webrtcshared.ConnectResponse{}, errors.Wrap(err, "unable to encode JSON offer")
	}
	resp, err := http.Post("http://"+ipAddress+"/sdp", "application/json; charset=utf-8", b)
	if err != nil {
		return webrtcshared.ConnectResponse{}, errors.Wrap(err, "unable to post JSON offer to SDP")
	}
	dec := json.NewDecoder(resp.Body)
	dec.DisallowUnknownFields()
	var connectResp webrtcshared.ConnectResponse
	err = dec.Decode(&connectResp)
	if err != nil {
		// ignore error returned if we failed to close this response body
		_ = resp.Body.Close()

		return webrtcshared.ConnectResponse{}, errors.Wrap(err, "decode response from SDP post")
	}
	if err := resp.Body.Close(); err != nil {
		return webrtcshared.ConnectResponse{}, errors.Wrap(err, "failed to close response stream from SDP post")
	}
	if len(connectResp.Candidates) == 0 {
		return webrtcshared.ConnectResponse{}, errors.New("missing candidates from connection, expected more than 0 candidates")
	}
	return connectResp, nil
}

func (client *Client) start() error {
	// Create a new RTCPeerConnection
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				// URLs of ICE servers (can be STUN or TURN but we just use STUN)
				// eg. []string{"stun:stun.l.google.com:19302"}
				URLs: client.options.ICEServerURLs,
			},
		},
	}

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return errors.Wrap(err, "unable to start peer connection")
	}

	// Create a datachannel with label 'data'
	dataChannel, err := peerConnection.CreateDataChannel("data", &webrtc.DataChannelInit{
		// NOTE(Jae): 2020-05-05
		// To force UDP mode
		// - ordered: false
		// - maxRetransmits: 0
		// Source: https://www.html5rocks.com/en/tutorials/webrtc/datachannels/
		Ordered:        new(bool),
		MaxRetransmits: new(uint16),
	})
	if err != nil {
		peerConnection.Close()
		dataChannel.Close()
		return errors.Wrap(err, "unable to create data channel")
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		switch connectionState {
		case webrtc.ICEConnectionStateClosed,
			webrtc.ICEConnectionStateFailed:
			client.close()
			// note(jae): 2021-04-15
			// explicitly don't handle "disconnected" state as this can be temporary
			// and change back to "connected" state in flaky networks
			// see: https://developer.mozilla.org/en-US/docs/Web/API/RTCPeerConnection/iceConnectionState
			//case webrtc.ICETransportStateDisconnected:
		}
	})

	// Create an offer to send to the server
	offer, err := peerConnection.CreateOffer(&webrtc.OfferOptions{
		// todo(Jae): 2021-28-02
		// Look into "ICERestart: true"
		//
		// As per release docs for webrtc/v3:
		// You can now initiate and accept an ICE Restart! This means that if a PeerConnection goes to Disconnected or
		// Failed because of network interruption it is no longer fatal.
		ICERestart: false,
	})
	if err != nil {
		peerConnection.Close()
		dataChannel.Close()
		return errors.Wrap(err, "unable to create offer")
	}

	// Sets the LocalDescription, and starts our UDP listeners
	if err := peerConnection.SetLocalDescription(offer); err != nil {
		peerConnection.Close()
		dataChannel.Close()
		return errors.Wrap(err, "unable to set local description")
	}

	// Exchange the SDP offer and answer using an HTTP Post request.
	connectResp, err := postConnect(client.options.IPAddress, offer)
	if err != nil {
		peerConnection.Close()
		dataChannel.Close()
		return err
	}

	// Register channel opening handling
	dataChannel.OnOpen(func() {
		client.mu.Lock()
		client.peerConnection = peerConnection
		client.dataChannel = dataChannel
		client.packets = make(chan []byte)
		client.mu.Unlock()

		// note(jae): 2021-04-15
		// this mixing of mutexes and atomics here for the connection state
		// is probably flakey unknown ways I don't yet understand.
		//
		// Future improvement might be to just put everything under a mutex
		// until I have more confidence/practice with atomics
		client.setIsConnected(true)
		client.setHasConnectedOnce(true)
	})

	// Note(jae): 2021-03-27
	// pions/webrtc WASM build is missing "dataChannel.OnError" implementation
	// and wont build
	//dataChannel.OnError(func(err error) {
	//	fmt.Printf("Data channel error: %+v", err)
	//})

	// Register text message handling
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		client.packets <- msg.Data
	})

	dataChannel.OnClose(func() {
		close(client.packets)
	})

	// Apply the answer as the remote description
	err = peerConnection.SetRemoteDescription(connectResp.Answer)
	if err != nil {
		peerConnection.Close()
		dataChannel.Close()
		return errors.Wrapf(err, "unable to set remote description: %v", connectResp.Answer)
	}
	for _, candidate := range connectResp.Candidates {
		if err := peerConnection.AddICECandidate(candidate); err != nil {
			peerConnection.Close()
			dataChannel.Close()
			return errors.Wrap(err, "unable to set remote description")
		}
	}

	return nil
}
