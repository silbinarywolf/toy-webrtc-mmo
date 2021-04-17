package rtt

import "testing"

type testCase struct {
	A      uint16
	B      uint16
	Output bool
}

var goldenTests = []testCase{
	{
		A:      1,
		B:      0,
		Output: true,
	},
	{
		A:      101,
		B:      100,
		Output: true,
	},
	{
		A:      65001,
		B:      65000,
		Output: true,
	},
	{
		A:      1,
		B:      65000,
		Output: true,
	},
	{
		A:      300,
		B:      65000,
		Output: true,
	},
}

// IsWrappedUInt16GreaterThan will test all the packet types to ensure writing / reading works
// for each registered struct
func TestIsWrappedUInt16GreaterThan(t *testing.T) {
	for _, test := range goldenTests {
		if res := IsWrappedUInt16GreaterThan(test.A, test.B); res != test.Output {
			t.Errorf("failed on input (%d, %d), returned %v but expected %v", test.A, test.B, res, test.Output)
		}
	}
}
