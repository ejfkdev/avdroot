package imgfmt

import (
	"bytes"
	"testing"
)

// All payloads must survive a compress/decompress round trip, and the format
// must be detected from the compressed bytes.
func TestRoundTrip(t *testing.T) {
	// Compressible data, plus something incompressible to exercise the
	// literal-only LZ4 block path.
	compressible := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog\n"), 3000)
	incompressible := make([]byte, 200000)
	for i := range incompressible {
		// A cheap pseudo-random sequence that no compressor will shrink.
		incompressible[i] = byte(i*2654435761>>13) ^ byte(i>>7)
	}

	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"compressible", compressible},
		{"incompressible", incompressible},
		{"empty", nil},
		{"tiny", []byte("x")},
	} {
		for _, format := range []Format{Gzip, XZ, LZMA, LZ4Legacy, LZ4} {
			encoded, err := Compress(format, tc.data)
			if err != nil {
				t.Errorf("%s/%s: compress: %v", tc.name, format, err)
				continue
			}
			decoded, detected, err := Decompress(encoded)
			if err != nil {
				t.Errorf("%s/%s: decompress: %v", tc.name, format, err)
				continue
			}
			if detected != format {
				t.Errorf("%s: detected %s, want %s", tc.name, detected, format)
			}
			if !bytes.Equal(decoded, tc.data) {
				t.Errorf("%s/%s: round trip changed the data (%d -> %d bytes)",
					tc.name, format, len(tc.data), len(decoded))
			}
		}
	}
}

// Detection must recognise each format and refuse arbitrary bytes.
func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want Format
	}{
		{"gzip", []byte{0x1f, 0x8b, 0x08, 0x00}, Gzip},
		{"gzip alt magic", []byte{0x1f, 0x9e, 0x00, 0x00}, Gzip},
		{"lz4 legacy", []byte{0x02, 0x21, 0x4c, 0x18}, LZ4Legacy},
		{"lz4 frame", []byte{0x04, 0x22, 0x4d, 0x18}, LZ4},
		{"lz4 frame old", []byte{0x03, 0x21, 0x4c, 0x18}, LZ4},
		{"xz", []byte{0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00}, XZ},
		{"bzip2", []byte("BZh9"), Bzip2},
		{"lzop", []byte{0x89, 0x4c, 0x5a, 0x4f}, Lzop},
		{"lzma", lzmaStream(), LZMA},
		{"unknown", []byte("this is not compressed"), Unknown},
	}
	for _, tc := range cases {
		if got := Detect(tc.data); got != tc.want {
			t.Errorf("Detect(%s) = %s, want %s", tc.name, got, tc.want)
		}
	}
}

// lzmaHeader builds the byte pattern Magisk's heuristic looks for: lc/lp/pb
// byte 0x5d, a power-of-two dictionary size, and eight 0xff bytes. The heuristic
// requires more than 13 bytes, so callers use lzmaStream.
func lzmaHeader() []byte {
	b := make([]byte, 13)
	b[0] = 0x5d
	// dictionary size 8 MiB (0x00800000), little endian
	b[1], b[2], b[3], b[4] = 0x00, 0x00, 0x80, 0x00
	for i := 5; i < 13; i++ {
		b[i] = 0xff
	}
	return b
}

// lzmaStream is an LZMA header followed by enough bytes to pass the length
// check in the heuristic.
func lzmaStream() []byte { return append(lzmaHeader(), 0x00, 0x00, 0x00, 0x00) }

// A non-power-of-two dictionary size means the bytes are not LZMA, which keeps
// the heuristic from misdetecting random data that happens to start with 0x5d.
func TestLZMAHeuristicRejectsBadDictionary(t *testing.T) {
	b := lzmaHeader()
	b[1] = 0x01 // dictionary size is no longer a power of two
	if got := Detect(b); got == LZMA {
		t.Error("Detect treated a bad dictionary size as LZMA")
	}
	if got := Detect(append([]byte{0x5d, 0x00, 0x00}, bytes.Repeat([]byte{0x00}, 20)...)); got == LZMA {
		t.Error("Detect treated short data as LZMA")
	}
}

// Compressing to bzip2 is not supported, and must fail clearly rather than
// writing a file the device cannot read.
func TestBzip2CompressUnsupported(t *testing.T) {
	if _, err := Compress(Bzip2, []byte("data")); err == nil {
		t.Fatal("expected bzip2 compression to be reported as unsupported")
	}
}

// LZ4 legacy output must start with the magic and hold a size-prefixed block,
// which is the layout the kernel and magiskboot expect.
func TestLZ4LegacyLayout(t *testing.T) {
	data := bytes.Repeat([]byte("payload"), 5000)
	out, err := compressLZ4Legacy(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 8 {
		t.Fatalf("output too short: %d bytes", len(out))
	}
	if got := uint32(out[0]) | uint32(out[1])<<8 | uint32(out[2])<<16 | uint32(out[3])<<24; got != lz4LegacyMagic {
		t.Errorf("magic = %#x, want %#x", got, lz4LegacyMagic)
	}
	// The magic is written once, then a single block for input under 8 MiB.
	size := uint32(out[4]) | uint32(out[5])<<8 | uint32(out[6])<<16 | uint32(out[7])<<24
	if int(size) != len(out)-8 {
		t.Errorf("block size %d does not account for the remaining %d bytes", size, len(out)-8)
	}
}

// A legacy stream may repeat the magic before later blocks; stock Android SDK
// ramdisks are built that way.
func TestLZ4LegacyHandlesRepeatedMagic(t *testing.T) {
	data := bytes.Repeat([]byte("abcdefgh"), 20000)
	single, err := compressLZ4Legacy(data)
	if err != nil {
		t.Fatal(err)
	}
	// Concatenate the same stream twice, which is what two back-to-back
	// archives look like on disk.
	doubled := append(append([]byte{}, single...), single...)

	got, err := decompressLZ4Legacy(doubled)
	if err != nil {
		t.Fatalf("decompressing a doubled stream: %v", err)
	}
	want := append(append([]byte{}, data...), data...)
	if !bytes.Equal(got, want) {
		t.Errorf("doubled stream decoded to %d bytes, want %d", len(got), len(want))
	}
}

// ParseFormat must accept the names users are likely to type.
func TestParseFormat(t *testing.T) {
	for name, want := range map[string]Format{
		"gzip": Gzip, "gz": Gzip,
		"lz4_legacy": LZ4Legacy, "lz4-legacy": LZ4Legacy, "lz4legacy": LZ4Legacy,
		"lz4": LZ4, "xz": XZ, "lzma": LZMA, "bzip2": Bzip2,
	} {
		got, err := ParseFormat(name)
		if err != nil {
			t.Errorf("ParseFormat(%q): %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("ParseFormat(%q) = %s, want %s", name, got, want)
		}
	}
	if _, err := ParseFormat("zstd"); err == nil {
		t.Error("expected an error for an unknown format")
	}
}
