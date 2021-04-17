package webrtcshared

import (
	"github.com/pion/webrtc/v3"
)

type ConnectResponse struct {
	Candidates []webrtc.ICECandidateInit `json:"candidates"`
	Answer     webrtc.SessionDescription `json:"answer"`
}
