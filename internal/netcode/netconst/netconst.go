// netconst are constants used by either the client and/server
package netconst

// MaxClientInputBuffer is how many frames of input we hold onto so that when we get world state from the
// server, we can replay our inputs that haven't been simulated by the server yet.
const MaxClientInputBuffer = 30

// MaxServerInputBuffer is how many frames of inputs we fire at the server per frame
//
// We only send the server our last N frames of input as we don't want the server
// to lag behind in bad network conditions.
//
// This value *kinda* says "in the worst network conditions, the server only has N frames to process and catch-up on"
//
// This value also used to be tied to "MaxClientInputBuffer" but when testing with
// "clumsy" (packet loss/latency tool for Windows) at 350ms ping, we got a less janky user experience on the
// client-side when we sent up only the last few frames but kept more for ourself
const MaxServerInputBuffer = 10
