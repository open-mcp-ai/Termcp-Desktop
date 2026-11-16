package transcript

import "testing"

// TestStreamCoordsRenumberOneStream is issue #202's arithmetic, without a channel.
//
// The channel is one interleaved write order and the stderr ranges are the index that
// separates it, so a stream's numbering is the channel's with the other stream's bytes
// put back. Both directions have to agree or paging in a stream's own coordinates lands
// somewhere else: `--stream e` hands the caller stderr offsets, and `--offset` has to
// take them back to the channel byte the reader will actually seek to.
func TestStreamCoordsRenumberOneStream(t *testing.T) {
	// The issue's second example: 7 bytes of stdout, then 30 of stderr.
	ranges := [][2]int64{{7, 37}}
	stderr := streamCoords{ranges: ranges, stderr: true}
	stdout := streamCoords{ranges: ranges, stderr: false}

	if got := stderr.pos(37); got != 30 {
		t.Errorf("stderr holds %d bytes of a 37 byte channel, want 30", got)
	}
	if got := stderr.pos(7); got != 0 {
		t.Errorf("the first stderr byte is at channel 7, so it is stderr byte 0, got %d", got)
	}
	if got := stderr.pos(1000); got != 30 {
		t.Errorf("a position past the channel is still 30 stderr bytes, got %d", got)
	}
	if got := stderr.channel(0); got != 7 {
		t.Errorf("stderr byte 0 lives at channel 7, got %d", got)
	}
	if got := stderr.channel(30); got != 37 {
		t.Errorf("one past the last stderr byte is channel 37, got %d", got)
	}
	if got := stdout.pos(37); got != 7 {
		t.Errorf("stdout holds 7 of those 37 bytes, got %d", got)
	}
	if got := stdout.channel(0); got != 0 {
		t.Errorf("the channel starts with stdout, so stdout byte 0 is channel 0, got %d", got)
	}
	if got := stdout.channel(7); got != 37 {
		t.Errorf("one past the last stdout byte is channel 37, got %d", got)
	}

	// An INTERLEAVED channel, which is the case the whole index exists for: the two
	// streams do not meet at a boundary, so any arithmetic that assumes they do (a
	// subtraction, a single split point) is wrong here.
	inter := [][2]int64{{2, 4}, {6, 8}}
	e := streamCoords{ranges: inter, stderr: true}
	o := streamCoords{ranges: inter, stderr: false}
	for n, want := range []int64{2, 3, 6, 7} {
		if got := e.channel(int64(n)); got != want {
			t.Errorf("stderr byte %d is channel %d, got %d", n, want, got)
		}
		if got := e.pos(want); got != int64(n) {
			t.Errorf("channel %d is stderr byte %d, got %d", want, n, got)
		}
	}
	if got := o.channel(0); got != 0 {
		t.Errorf("stdout byte 0 is channel 0, got %d", got)
	}
	if got := o.channel(2); got != 4 {
		t.Errorf("stdout byte 2 is channel 4 (the gap after the first stderr run), got %d", got)
	}
	if got := o.channel(4); got != 8 {
		t.Errorf("stdout byte 4 is channel 8, got %d", got)
	}

	// A stderr run that STARTS AT ZERO: stdout's first byte is then after it.
	lead := [][2]int64{{0, 5}}
	if got := (streamCoords{ranges: lead, stderr: false}).channel(0); got != 5 {
		t.Errorf("with stderr first, stdout byte 0 is channel 5, got %d", got)
	}
	if got := (streamCoords{ranges: lead, stderr: false}).pos(5); got != 0 {
		t.Errorf("channel 5 is stdout byte 0, got %d", got)
	}

	// NO STDERR AT ALL: the stream is empty, and every address in it is 0. That is the
	// number the issue wanted — `*/0` says "this stream is empty", which `*/7` could
	// not, because 7 was the channel's length.
	none := streamCoords{ranges: nil, stderr: true}
	if got := none.pos(21); got != 0 {
		t.Errorf("a channel with no stderr has 0 stderr bytes, got %d", got)
	}
	if got := none.channel(0); got != 0 {
		t.Errorf("the 0th byte of an empty stream is position 0, got %d", got)
	}
	all := streamCoords{ranges: nil, stderr: false}
	if got := all.pos(21); got != 21 {
		t.Errorf("with no stderr the whole channel is stdout, got %d", got)
	}
}

// TestStreamCoordsRoundTrip is the property the two directions exist for.
//
// Paging is: read in stream coordinates, resume at the number the reply gave. That only
// works if translating the number forward (stream → channel) and then back
// (channel → stream) is the identity, for every address a reply can carry.
func TestStreamCoordsRoundTrip(t *testing.T) {
	cases := [][][2]int64{
		nil,
		{{0, 5}},
		{{7, 37}},
		{{2, 4}, {6, 8}},
		{{0, 1}, {3, 5}, {7, 8}, {10, 20}},
	}
	for _, ranges := range cases {
		for _, stderr := range []bool{true, false} {
			c := streamCoords{ranges: ranges, stderr: stderr}
			total := c.pos(1 << 20) // more than any range here
			for n := int64(0); n <= total; n++ {
				at := c.channel(n)
				if got := c.pos(at); got != n {
					t.Fatalf("ranges %v stderr %v: stream byte %d is channel %d, which maps back to %d",
						ranges, stderr, n, at, got)
				}
			}
		}
	}
}
