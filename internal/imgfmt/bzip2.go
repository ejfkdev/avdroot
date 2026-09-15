package imgfmt

import (
	"bytes"
	"compress/bzip2"
	"io"
)

// decompressBzip2 uses the standard library, which supports reading bzip2 but
// not writing it; ramdisks in the wild are read-only for this format, so that
// asymmetry is acceptable (magiskboot would use a third-party encoder).
func decompressBzip2(data []byte) ([]byte, error) {
	return io.ReadAll(bzip2.NewReader(bytes.NewReader(data)))
}
