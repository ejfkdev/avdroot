// Package imgfmt handles the compression formats used for Android ramdisks,
// matching the set that Magisk's magiskboot can read and write.
//
// Detection mirrors magiskboot's check_fmt order and magic constants; LZ4 is
// implemented at block level so that the "legacy" stream written by the
// Android SDK (magic 0x184C2102) round-trips byte-compatibly.
package imgfmt

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/pierrec/lz4/v4"
	"github.com/ulikunitz/xz"
	xzlzma "github.com/ulikunitz/xz/lzma"
)

// Format identifies a compression container.
type Format int

const (
	Unknown Format = iota
	Gzip
	XZ
	LZMA
	Bzip2
	LZ4 // modern LZ4 frame
	LZ4Legacy
	Lzop // detected but not supported for (de)compression
)

func (f Format) String() string {
	switch f {
	case Gzip:
		return "gzip"
	case XZ:
		return "xz"
	case LZMA:
		return "lzma"
	case Bzip2:
		return "bzip2"
	case LZ4:
		return "lz4"
	case LZ4Legacy:
		return "lz4_legacy"
	case Lzop:
		return "lzop"
	default:
		return "unknown"
	}
}

// ParseFormat maps a user supplied name onto a Format. Both magiskboot's and
// the conventional names are accepted.
func ParseFormat(s string) (Format, error) {
	switch s {
	case "gzip", "gz":
		return Gzip, nil
	case "xz":
		return XZ, nil
	case "lzma":
		return LZMA, nil
	case "bzip2", "bz2":
		return Bzip2, nil
	case "lz4":
		return LZ4, nil
	case "lz4_legacy", "lz4-legacy", "lz4legacy":
		return LZ4Legacy, nil
	default:
		return Unknown, fmt.Errorf("unsupported compression format %q", s)
	}
}

// magiskboot's magic constants.
var (
	magicGzip1     = []byte{0x1f, 0x8b}
	magicGzip2     = []byte{0x1f, 0x9e}
	magicLzop      = []byte{0x89, 0x4c, 0x5a, 0x4f}
	magicXZ        = []byte{0xfd, 0x37, 0x7a, 0x58, 0x5a}
	magicBzip2     = []byte("BZh")
	magicLZ4Frame1 = []byte{0x03, 0x21, 0x4c, 0x18}
	magicLZ4Frame2 = []byte{0x04, 0x22, 0x4d, 0x18}
	magicLZ4Legacy = []byte{0x02, 0x21, 0x4c, 0x18}

	// lz4LegacyMagic is the block-stream magic this package encodes; the modern
	// frame magic is handled by the frame decoder and needs no constant.
	lz4LegacyMagic uint32 = 0x184C2102
)

// lz4BlockSize is the uncompressed block size magiskboot uses for legacy
// streams (0x800000).
const lz4BlockSize = 0x800000

// Detect returns the compression format of data.
func Detect(data []byte) Format {
	switch {
	case hasPrefix(data, magicGzip1), hasPrefix(data, magicGzip2):
		return Gzip
	case hasPrefix(data, magicLzop):
		return Lzop
	case hasPrefix(data, magicXZ):
		return XZ
	case guessLZMA(data):
		return LZMA
	case hasPrefix(data, magicBzip2):
		return Bzip2
	case hasPrefix(data, magicLZ4Frame1), hasPrefix(data, magicLZ4Frame2):
		return LZ4
	case hasPrefix(data, magicLZ4Legacy):
		return LZ4Legacy
	default:
		return Unknown
	}
}

func hasPrefix(b, prefix []byte) bool {
	return len(b) >= len(prefix) && bytes.Equal(b[:len(prefix)], prefix)
}

// guessLZMA ports magiskboot's heuristic: a valid LZMA (alone) header starts
// with the lc/lp/pb byte 0x5d, a power-of-two dictionary size, and eight 0xff
// bytes where the uncompressed size would go.
func guessLZMA(b []byte) bool {
	if len(b) <= 13 || b[0] != 0x5d {
		return false
	}
	dict := binary.LittleEndian.Uint32(b[1:5])
	if dict == 0 || dict&(dict-1) != 0 {
		return false
	}
	return bytes.Equal(b[5:13], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
}

// IsCompressed reports whether f denotes a format that can be decompressed.
func (f Format) IsCompressed() bool {
	switch f {
	case Gzip, XZ, LZMA, Bzip2, LZ4, LZ4Legacy:
		return true
	default:
		return false
	}
}

// ErrUnsupportedFormat is returned for formats we can detect but not process.
var ErrUnsupportedFormat = errors.New("imgfmt: unsupported compression format")

// Decompress detects the format of data and returns the decompressed payload
// together with the format it found.
func Decompress(data []byte) ([]byte, Format, error) {
	f := Detect(data)
	if !f.IsCompressed() {
		if f == Lzop {
			return nil, f, fmt.Errorf("%w: lzop", ErrUnsupportedFormat)
		}
		return nil, f, fmt.Errorf("%w: %q is not a supported compression format", ErrUnsupportedFormat, firstBytes(data))
	}
	out, err := DecompressAs(f, data)
	return out, f, err
}

func firstBytes(b []byte) string {
	n := min(len(b), 8)
	return fmt.Sprintf("% x", b[:n])
}

// DecompressAs decompresses data using an explicit format.
func DecompressAs(f Format, data []byte) ([]byte, error) {
	switch f {
	case Gzip:
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		r.Multistream(true) // ramdisks may hold concatenated gzip members
		defer r.Close()
		return io.ReadAll(r)
	case XZ:
		r, err := xz.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(r)
	case LZMA:
		r, err := xzlzma.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(r)
	case Bzip2:
		return decompressBzip2(data)
	case LZ4:
		return decompressLZ4Frame(data)
	case LZ4Legacy:
		return decompressLZ4Legacy(data)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, f)
	}
}

// Compress encodes data in the requested format.
func Compress(f Format, data []byte) ([]byte, error) {
	switch f {
	case Gzip:
		var buf bytes.Buffer
		w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case XZ:
		// magiskboot uses preset 6 with a CRC32 check.
		var buf bytes.Buffer
		cfg := xz.WriterConfig{CheckSum: xz.CRC32}
		w, err := cfg.NewWriter(&buf)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case LZMA:
		var buf bytes.Buffer
		w, err := xzlzma.NewWriter(&buf)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case LZ4Legacy:
		return compressLZ4Legacy(data)
	case LZ4:
		return compressLZ4Frame(data)
	case Bzip2:
		return nil, fmt.Errorf("%w: bzip2 compression is not implemented", ErrUnsupportedFormat)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, f)
	}
}

// --- LZ4 legacy ---
//
// Layout: the legacy magic, then for each block a little-endian uint32 holding
// the compressed size followed by a raw LZ4 block. magiskboot reads up to 8 MiB
// of input per block.
//
// The magic may reappear before any block, because independent legacy streams
// can simply be concatenated (stock Android SDK ramdisks do exactly this: one
// stream per cpio archive). It is unambiguous as a block size, since
// 0x184C2102 far exceeds the 8 MiB block bound, so it is skipped wherever it
// occurs.

func decompressLZ4Legacy(data []byte) ([]byte, error) {
	pos := 0
	// The first field must be the legacy magic.
	if len(data) < 4 || binary.LittleEndian.Uint32(data) != lz4LegacyMagic {
		return nil, errors.New("imgfmt: not an lz4 legacy stream")
	}
	var out bytes.Buffer
	buf := make([]byte, lz4BlockSize)
	for pos+4 <= len(data) {
		size := binary.LittleEndian.Uint32(data[pos:])
		if size == lz4LegacyMagic {
			pos += 4
			continue
		}
		pos += 4
		if size == 0 || pos+int(size) > len(data) {
			// Not a block header: end of stream. This also absorbs the
			// lz4_lg uncompressed-size trailer and any trailing padding.
			break
		}
		n, err := lz4.UncompressBlock(data[pos:pos+int(size)], buf)
		if err != nil {
			return nil, fmt.Errorf("imgfmt: lz4 block at offset %d: %w", pos, err)
		}
		if n == 0 {
			return nil, fmt.Errorf("imgfmt: lz4 block at offset %d did not decompress", pos)
		}
		out.Write(buf[:n])
		pos += int(size)
	}
	return out.Bytes(), nil
}

func compressLZ4Legacy(data []byte) ([]byte, error) {
	var out bytes.Buffer
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], lz4LegacyMagic)
	out.Write(hdr[:])

	for off := 0; off < len(data); off += lz4BlockSize {
		end := min(off+lz4BlockSize, len(data))
		chunk := data[off:end]
		enc := make([]byte, lz4.CompressBlockBound(len(chunk)))
		n, err := lz4.CompressBlockHC(chunk, enc, lz4.Level9, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("imgfmt: lz4 compress: %w", err)
		}
		if n == 0 {
			// Incompressible input: emit a literal-only block, which is a
			// valid LZ4 block encoding of the raw bytes.
			enc = encodeLiteralOnly(chunk)
			n = len(enc)
		}
		binary.LittleEndian.PutUint32(hdr[:], uint32(n))
		out.Write(hdr[:])
		out.Write(enc[:n])
	}
	return out.Bytes(), nil
}

// encodeLiteralOnly builds an LZ4 block consisting of a single literal run.
func encodeLiteralOnly(src []byte) []byte {
	l := len(src)
	out := make([]byte, 0, l+l/255+16)
	token := byte(0)
	if l >= 15 {
		token = 15 << 4
	} else {
		token = byte(l) << 4
	}
	out = append(out, token)
	if l >= 15 {
		rem := l - 15
		for rem >= 255 {
			out = append(out, 255)
			rem -= 255
		}
		out = append(out, byte(rem))
	}
	return append(out, src...)
}

// --- LZ4 frame ---

func decompressLZ4Frame(data []byte) ([]byte, error) {
	r := lz4.NewReader(bytes.NewReader(data))
	return io.ReadAll(r)
}

func compressLZ4Frame(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := lz4.NewWriter(&buf)
	if err := w.Apply(
		lz4.BlockSizeOption(lz4.Block4Mb),
		lz4.BlockChecksumOption(true),
		lz4.ChecksumOption(true),
		lz4.CompressionLevelOption(lz4.Level9),
	); err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
