package rtt

import (
	"time"
)

// todo(jae): 2021-04-02
// Rename this to AckTracker, ack.Tracker?
// PacketSequencer?
//
// As it tracks packet sequences and acknowledgements of
// those packet sequences.

const (
	// roundTripTimeLimit is the amount of time that we store sent packets
	// which we use to determine the RTT / Latency between server/client.
	//
	// After reading blog posts by GafferonGames, he recommends to hold these for 1 second, as if those packets
	// haven't been acknolwedged after a second, they're most likely wildly out of date anyway and not helpful
	// for measuring RTT.
	//
	// Source:
	// https://gafferongames.com/post/reliability_ordering_and_congestion_avoidance_over_udp/#measuring-round-trip-time
	//
	maximumRoundTripTimeLimit = 1000 // in milliseconds

	maxFramerate = 60

	// roundTripTimeLimitInFrames is used to setup the fixed-size array for storing
	roundTripTimeLimitInFrames = (maximumRoundTripTimeLimit / 1000) * maxFramerate
)

type RoundTripTracking struct {
	packetSequenceID   uint16
	packetSequenceList [roundTripTimeLimitInFrames]packetSequence
	latency            time.Duration
}

type packetSequence struct {
	SequenceID uint16
	Time       time.Time
}

// Latency will return the smoothed average latency based on acknowledged
// packets
func (rtt *RoundTripTracking) Latency() time.Duration {
	return rtt.latency
}

func (rtt *RoundTripTracking) Next() uint16 {
	seqID := rtt.packetSequenceID
	rtt.packetSequenceID++

	// Store current time in free slot (anything older than 1000ms)
	now := time.Now()
	for i, _ := range rtt.packetSequenceList {
		sequence := &rtt.packetSequenceList[i]
		if now.Sub(sequence.Time).Milliseconds() > maximumRoundTripTimeLimit {
			sequence.SequenceID = seqID
			sequence.Time = now
			break
		}
	}
	return seqID
}

func (rtt *RoundTripTracking) Ack(seqID uint16) {
	var foundSequence *packetSequence
	for i := range rtt.packetSequenceList {
		sequence := &rtt.packetSequenceList[i]
		if sequence.SequenceID == seqID {
			foundSequence = sequence
			break
		}
	}
	if foundSequence == nil {
		// If the sequence expired or is too old, we ignore it
		return
	}
	timePacketSent := time.Since(foundSequence.Time)
	if rtt.latency == 0 {
		rtt.latency = timePacketSent
	} else {
		rtt.latency = time.Duration(float64(rtt.latency) + (0.10 * float64(timePacketSent-rtt.latency)))
	}
}

// IsWrappedUInt16GreaterThan checks to see if a is greater than b but accounts
// for overflowing numbers.
//
// This means that:
// - If a = 101 and b = 100, then a is greater than b.
// - If a = 1 and b is 65000, then a is greater than b. (as its overflowed and looped)
//
// Source: https://gafferongames.com/post/reliability_ordering_and_congestion_avoidance_over_udp/
func IsWrappedUInt16GreaterThan(s1 uint16, s2 uint16) bool {
	return ((s1 > s2) && (s1-s2 <= 32768)) ||
		((s1 < s2) && (s2-s1 > 32768))
}
