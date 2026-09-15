// Package axml reads the binary AndroidManifest.xml stored inside an APK.
//
// Only a handful of values are needed — the package name and the API level
// range the app declares — but they are worth reading from the manifest rather
// than guessed, because they decide whether an app can be installed at all.
//
// The format is Android's "AXML": a chunked container holding a string pool
// followed by XML nodes whose names and values are string-pool indices. See
// frameworks/base/libs/androidfw/include/androidfw/ResourceTypes.h.
package axml

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf16"
)

// Chunk types.
const (
	chunkStringPool   = 0x0001
	chunkResourceMap  = 0x0180
	chunkStartElement = 0x0102
)

// Attribute value types we care about.
const (
	typeString = 0x03
	typeIntDec = 0x10
	typeIntHex = 0x12
)

// String pool flags.
const (
	flagSorted = 1 << 0
	flagUTF8   = 1 << 8
)

// Android resource ids for the attributes of interest, used as a fallback when
// a name is missing from the string pool.
const (
	attrMinSdkVersion    = 0x0101020c
	attrTargetSdkVersion = 0x01010270
)

// ErrNotAXML is returned when the data is not a binary manifest.
var ErrNotAXML = errors.New("axml: not a binary AndroidManifest.xml")

// Manifest holds the values avdroot needs from an APK.
type Manifest struct {
	Package     string
	VersionName string
	VersionCode int64
	// MinSDK is the lowest API level the app can be installed on, or 0 when
	// the manifest does not say.
	MinSDK int
	// TargetSDK is the API level the app was built against, or 0 when unstated.
	TargetSDK int
}

// Parse reads a binary manifest.
func Parse(data []byte) (*Manifest, error) {
	if len(data) < 8 {
		return nil, ErrNotAXML
	}
	// File header: type 0x0003, header size 8, total size.
	if binary.LittleEndian.Uint16(data[0:2]) != 0x0003 {
		return nil, ErrNotAXML
	}
	headerSize := int(binary.LittleEndian.Uint16(data[2:4]))
	fileSize := int(binary.LittleEndian.Uint32(data[4:8]))
	if fileSize > 0 && fileSize <= len(data) {
		data = data[:fileSize]
	}

	pos := headerSize
	var pool *stringPool
	m := &Manifest{}

	for pos+8 <= len(data) {
		ctype := binary.LittleEndian.Uint16(data[pos : pos+2])
		chunkHeaderSize := int(binary.LittleEndian.Uint16(data[pos+2 : pos+4]))
		chunkSize := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		if chunkSize < 8 || pos+chunkSize > len(data) {
			break
		}
		chunk := data[pos : pos+chunkSize]

		switch ctype {
		case chunkStringPool:
			p, err := parseStringPool(chunk)
			if err != nil {
				return nil, err
			}
			pool = p
		case chunkStartElement:
			if pool == nil {
				return nil, fmt.Errorf("axml: element before string pool")
			}
			if err := parseElement(chunk, chunkHeaderSize, pool, m); err != nil {
				return nil, err
			}
		case chunkResourceMap:
			// Only needed to resolve attribute names by id; the name strings
			// are present in practice, and ids are matched directly below.
		}
		pos += chunkSize
	}
	return m, nil
}

// stringPool decodes the chunk of string data.
type stringPool struct {
	strings []string
	utf8    bool
}

func parseStringPool(chunk []byte) (*stringPool, error) {
	if len(chunk) < 28 {
		return nil, fmt.Errorf("axml: string pool too short")
	}
	count := int(binary.LittleEndian.Uint32(chunk[8:12]))
	flags := binary.LittleEndian.Uint32(chunk[16:20])
	stringsStart := int(binary.LittleEndian.Uint32(chunk[20:24]))
	if count < 0 || 28+count*4 > len(chunk) {
		return nil, fmt.Errorf("axml: string pool claims %d strings, which does not fit", count)
	}
	offsets := chunk[28 : 28+count*4]
	base := stringsStart
	if base < 0 || base > len(chunk) {
		return nil, fmt.Errorf("axml: string data offset out of range")
	}
	data := chunk[base:]

	sp := &stringPool{strings: make([]string, count), utf8: flags&flagUTF8 != 0}
	for i := 0; i < count; i++ {
		off := int(binary.LittleEndian.Uint32(offsets[i*4 : i*4+4]))
		if off < 0 || off >= len(data) {
			sp.strings[i] = ""
			continue
		}
		if sp.utf8 {
			sp.strings[i] = decodeUTF8String(data[off:])
		} else {
			sp.strings[i] = decodeUTF16String(data[off:])
		}
	}
	return sp, nil
}

// decodeLength reads the variable-length length prefix used by both encodings:
// a high bit in the first byte means the length needs two bytes.
func decodeLength(b []byte, wide bool) (length, consumed int, ok bool) {
	if wide {
		if len(b) < 2 {
			return 0, 0, false
		}
		v := int(binary.LittleEndian.Uint16(b[:2]))
		if v&0x8000 != 0 {
			if len(b) < 4 {
				return 0, 0, false
			}
			v = (v&0x7fff)<<16 | int(binary.LittleEndian.Uint16(b[2:4]))
			return v, 4, true
		}
		return v, 2, true
	}
	if len(b) < 1 {
		return 0, 0, false
	}
	if b[0]&0x80 != 0 {
		if len(b) < 2 {
			return 0, 0, false
		}
		return (int(b[0]&0x7f) << 8) | int(b[1]), 2, true
	}
	return int(b[0]), 1, true
}

func decodeUTF8String(b []byte) string {
	// UTF-8 entries carry the character count first, then the byte count.
	_, n1, ok := decodeLength(b, false)
	if !ok {
		return ""
	}
	byteLen, n2, ok := decodeLength(b[n1:], false)
	if !ok {
		return ""
	}
	start := n1 + n2
	if start+byteLen > len(b) {
		return ""
	}
	return string(b[start : start+byteLen])
}

func decodeUTF16String(b []byte) string {
	charLen, n, ok := decodeLength(b, true)
	if !ok {
		return ""
	}
	start := n
	end := start + charLen*2
	if end > len(b) {
		return ""
	}
	u := make([]uint16, charLen)
	for i := 0; i < charLen; i++ {
		u[i] = binary.LittleEndian.Uint16(b[start+i*2 : start+i*2+2])
	}
	return string(utf16.Decode(u))
}

// parseElement handles a start-element chunk, recording the attributes of the
// manifest and uses-sdk elements.
func parseElement(chunk []byte, headerSize int, pool *stringPool, m *Manifest) error {
	// ResXMLTree_node header, then ResXMLTree_attrExt.
	if len(chunk) < headerSize+20 {
		return nil
	}
	ext := chunk[headerSize:]
	nameIdx := int(binary.LittleEndian.Uint32(ext[4:8]))
	attrStart := int(binary.LittleEndian.Uint16(ext[8:10]))
	attrSize := int(binary.LittleEndian.Uint16(ext[10:12]))
	attrCount := int(binary.LittleEndian.Uint16(ext[12:14]))
	if attrSize == 0 {
		attrSize = 20
	}
	name := pool.at(nameIdx)

	// Attributes begin at the end of the node header plus attrStart.
	base := headerSize + attrStart
	if base < 0 || base > len(chunk) {
		return nil
	}
	for i := 0; i < attrCount; i++ {
		off := base + i*attrSize
		if off+attrSize > len(chunk) || attrSize < 20 {
			break
		}
		a := chunk[off : off+attrSize]
		nameStrIdx := int(binary.LittleEndian.Uint32(a[4:8]))
		valueType := a[15]
		dataVal := binary.LittleEndian.Uint32(a[16:20])
		attrName := pool.at(nameStrIdx)
		resID := binary.LittleEndian.Uint32(a[0:4])

		switch name {
		case "manifest":
			switch {
			case attrName == "package":
				m.Package = pool.at(int(dataVal))
			case attrName == "versionName":
				m.VersionName = pool.at(int(dataVal))
			case attrName == "versionCode" && isInt(valueType):
				m.VersionCode = int64(int32(dataVal))
			}
		case "uses-sdk":
			switch {
			case attrName == "minSdkVersion" || resID == attrMinSdkVersion:
				if isInt(valueType) {
					m.MinSDK = int(dataVal)
				}
			case attrName == "targetSdkVersion" || resID == attrTargetSdkVersion:
				if isInt(valueType) {
					m.TargetSDK = int(dataVal)
				}
			}
		}
	}
	return nil
}

func isInt(t byte) bool { return t == typeIntDec || t == typeIntHex }

func (sp *stringPool) at(i int) string {
	if i < 0 || i >= len(sp.strings) {
		return ""
	}
	return sp.strings[i]
}
