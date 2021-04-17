package netconf

type Options struct {
	// PublicIP is used by the:
	// Client: to connect to server
	// Server: to setup the STUN server
	PublicIP string
}
