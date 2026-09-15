// Package cpio implements the "newc" cpio archive format as written by
// Magisk's magiskboot, together with the Magisk-specific ramdisk operations
// (status test, fstab patching, backup and restore).
//
// The serialisation is byte-for-byte compatible with magiskboot from Magisk
// v31: every field is normalised on write (inode counting from 300000, nlink
// fixed at 1, mtime 0, devmajor/devminor 0) and entries are emitted in
// byte-wise sorted name order. Ramdisks produced by this package are
// therefore indistinguishable from ones produced by the official installer.
package cpio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// File type bits (st_mode & S_IFMT).
const (
	S_IFMT  = 0o170000
	S_IFREG = 0o100000
	S_IFDIR = 0o040000
	S_IFLNK = 0o120000
	S_IFBLK = 0o060000
	S_IFCHR = 0o020000
)

// headerSize is the fixed size of a newc header: 6 magic bytes plus thirteen
// 8-digit hexadecimal fields.
const headerSize = 110

const (
	magic     = "070701"
	trailer   = "TRAILER!!!"
	inodeBase = 300000
)

// Entry is a single member of a cpio archive.
type Entry struct {
	Mode      uint32
	UID       uint32
	GID       uint32
	RdevMajor uint32
	RdevMinor uint32
	Data      []byte
}

// IsRegular reports whether the entry is a regular file.
func (e *Entry) IsRegular() bool { return e.Mode&S_IFMT == S_IFREG }

// IsDir reports whether the entry is a directory.
func (e *Entry) IsDir() bool { return e.Mode&S_IFMT == S_IFDIR }

// IsSymlink reports whether the entry is a symbolic link.
func (e *Entry) IsSymlink() bool { return e.Mode&S_IFMT == S_IFLNK }

// Perm returns the permission bits without the file type.
func (e *Entry) Perm() uint32 { return e.Mode &^ S_IFMT }

func (e *Entry) clone() *Entry {
	c := *e
	if e.Data != nil {
		c.Data = append([]byte(nil), e.Data...)
	}
	return &c
}

// Archive is an ordered-by-name collection of cpio entries.
type Archive struct {
	entries map[string]*Entry
}

// New returns an empty archive.
func New() *Archive {
	return &Archive{entries: make(map[string]*Entry)}
}

// NormalizePath collapses redundant slashes so that "/init", "init",
// "//init" and "./init" all denote the same entry. It mirrors magiskboot's
// norm_path helper: split on '/', drop empty components, rejoin.
func NormalizePath(p string) string {
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, "/")
}

func align4(n int) int { return (n + 3) &^ 3 }

// Load parses a newc archive. Concatenated archives (as found in Android 11+
// ramdisks, where a generic ramdisk and a vendor ramdisk are appended) are
// merged into one archive; on name collisions the later entry wins, matching
// both the kernel's sequential unpacking and magiskboot's reader.
func Load(data []byte) (*Archive, error) {
	a := New()
	pos := 0
	for pos < len(data) {
		if pos+headerSize > len(data) {
			// Trailing bytes too short to hold a header; stop rather than
			// failing, since Android ramdisks are padded to page size.
			break
		}
		raw := data[pos : pos+headerSize]
		if string(raw[:6]) != magic {
			// Resynchronise: some images carry alignment padding between
			// concatenated archives.
			next := bytes.Index(data[pos:], []byte(magic))
			if next < 0 {
				break
			}
			pos += next
			continue
		}
		h, err := parseHeader(raw)
		if err != nil {
			return nil, err
		}
		pos += headerSize

		nameSize := int(h.Namesize)
		if pos+nameSize > len(data) {
			return nil, fmt.Errorf("cpio: name field truncated at offset %d", pos)
		}
		nameBytes := data[pos : pos+nameSize]
		if i := bytes.IndexByte(nameBytes, 0); i >= 0 {
			nameBytes = nameBytes[:i]
		}
		name := string(nameBytes)
		pos = align4(pos + nameSize)

		switch name {
		case ".", "..":
			// Skip explicitly; real archives give these a zero length, but
			// honouring Filesize keeps us in sync on unusual images.
			pos = align4(pos + int(h.Filesize))
			continue
		case trailer:
			// Another archive may follow. magiskboot scans forward for the
			// next header magic and continues.
			next := bytes.Index(data[pos:], []byte(magic))
			if next < 0 {
				return a, nil
			}
			pos += next
			continue
		}

		if pos+int(h.Filesize) > len(data) {
			return nil, fmt.Errorf("cpio: %q data truncated at offset %d", name, pos)
		}
		dataCopy := make([]byte, h.Filesize)
		copy(dataCopy, data[pos:pos+int(h.Filesize)])
		pos = align4(pos + int(h.Filesize))

		a.entries[name] = &Entry{
			Mode:      h.Mode,
			UID:       h.UID,
			GID:       h.GID,
			RdevMajor: h.RdevMajor,
			RdevMinor: h.RdevMinor,
			Data:      dataCopy,
		}
	}
	return a, nil
}

type header struct {
	Mode                 uint32
	UID, GID             uint32
	Filesize             uint32
	RdevMajor, RdevMinor uint32
	Namesize             uint32
}

// field decodes one 8-character hexadecimal field.
func field(b []byte) (uint32, error) {
	var v uint32
	for _, c := range b {
		var d uint32
		switch {
		case c >= '0' && c <= '9':
			d = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint32(c-'A') + 10
		default:
			return 0, fmt.Errorf("cpio: invalid hex byte %q", c)
		}
		v = v<<4 | d
	}
	return v, nil
}

func parseHeader(b []byte) (header, error) {
	var h header
	var err error
	// Layout: magic(6) ino(8) mode(8) uid(8) gid(8) nlink(8) mtime(8)
	//         filesize(8) devmajor(8) devminor(8) rdevmajor(8) rdevminor(8)
	//         namesize(8) check(8)
	if h.Mode, err = field(b[14:22]); err != nil {
		return h, err
	}
	if h.UID, err = field(b[22:30]); err != nil {
		return h, err
	}
	if h.GID, err = field(b[30:38]); err != nil {
		return h, err
	}
	if h.Filesize, err = field(b[54:62]); err != nil {
		return h, err
	}
	if h.RdevMajor, err = field(b[78:86]); err != nil {
		return h, err
	}
	if h.RdevMinor, err = field(b[86:94]); err != nil {
		return h, err
	}
	if h.Namesize, err = field(b[94:102]); err != nil {
		return h, err
	}
	return h, nil
}

// Dump serialises the archive in magiskboot's normalised form.
func (a *Archive) Dump(w io.Writer) error {
	bw := &countingWriter{w: w}
	inode := uint32(inodeBase)
	for _, name := range a.Names() {
		e := a.entries[name]
		if err := bw.header(inode, e.Mode, e.UID, e.GID, uint32(len(e.Data)),
			e.RdevMajor, e.RdevMinor, uint32(len(name)+1)); err != nil {
			return err
		}
		if _, err := bw.Write([]byte(name)); err != nil {
			return err
		}
		if _, err := bw.Write([]byte{0}); err != nil {
			return err
		}
		if err := bw.pad(); err != nil {
			return err
		}
		if _, err := bw.Write(e.Data); err != nil {
			return err
		}
		if err := bw.pad(); err != nil {
			return err
		}
		inode++
	}
	// Trailer: mode 0755 with no file type bits, namesize 11.
	if err := bw.header(inode, 0o755, 0, 0, 0, 0, 0, uint32(len(trailer)+1)); err != nil {
		return err
	}
	if _, err := bw.Write([]byte(trailer)); err != nil {
		return err
	}
	if _, err := bw.Write([]byte{0}); err != nil {
		return err
	}
	return bw.pad()
}

// Bytes serialises the archive to a byte slice.
func (a *Archive) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := a.Dump(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// countingWriter keeps track of the absolute output offset so that data and
// name fields can be padded to a 4-byte boundary exactly like magiskboot,
// whose alignment depends on the absolute file position.
type countingWriter struct {
	w   io.Writer
	pos int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.pos += n
	return n, err
}

func (c *countingWriter) pad() error {
	if n := align4(c.pos) - c.pos; n > 0 {
		_, err := c.Write(make([]byte, n))
		return err
	}
	return nil
}

func (c *countingWriter) header(inode, mode, uid, gid, filesize, rdevMajor, rdevMinor, namesize uint32) error {
	// Field order matches magiskboot: ino mode uid gid nlink mtime filesize
	// devmajor devminor rdevmajor rdevminor namesize check.
	h := fmt.Sprintf("%s%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x",
		magic, inode, mode, uid, gid, 1, 0, filesize, 0, 0,
		rdevMajor, rdevMinor, namesize, 0)
	_, err := c.Write([]byte(h))
	return err
}

// Names returns all entry names in byte-wise sorted order.
func (a *Archive) Names() []string {
	names := make([]string, 0, len(a.entries))
	for n := range a.entries {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Len returns the number of entries.
func (a *Archive) Len() int { return len(a.entries) }

// Get returns the entry with the given name.
func (a *Archive) Get(name string) (*Entry, bool) {
	e, ok := a.entries[NormalizePath(name)]
	return e, ok
}

// Exists reports whether an entry is present.
func (a *Archive) Exists(name string) bool {
	_, ok := a.entries[NormalizePath(name)]
	return ok
}

// Put inserts or replaces an entry under a normalised name.
func (a *Archive) Put(name string, e *Entry) {
	a.entries[NormalizePath(name)] = e
}

// AddFile stores data as a regular file with the given permission bits,
// mirroring magiskboot's "add MODE ENTRY INFILE".
func (a *Archive) AddFile(perm uint32, name string, data []byte) {
	a.Put(name, &Entry{Mode: perm | S_IFREG, Data: data})
}

// Mkdir creates a directory entry, mirroring magiskboot's "mkdir MODE ENTRY".
func (a *Archive) Mkdir(perm uint32, name string) {
	a.Put(name, &Entry{Mode: perm | S_IFDIR})
}

// Symlink creates a symbolic link entry, mirroring magiskboot's "ln TARGET ENTRY".
// Note that magiskboot sets only S_IFLNK and no permission bits.
func (a *Archive) Symlink(target, name string) {
	a.Put(name, &Entry{Mode: S_IFLNK, Data: []byte(NormalizePath(target))})
}

// Rm removes an entry, and with recursive set also everything beneath it.
func (a *Archive) Rm(name string, recursive bool) {
	path := NormalizePath(name)
	delete(a.entries, path)
	if recursive {
		prefix := path + "/"
		for k := range a.entries {
			if strings.HasPrefix(k, prefix) {
				delete(a.entries, k)
			}
		}
	}
}

// Clone returns a deep copy of the archive.
func (a *Archive) Clone() *Archive {
	c := New()
	for k, v := range a.entries {
		c.entries[k] = v.clone()
	}
	return c
}

// ErrNotFound is returned when a requested entry does not exist.
var ErrNotFound = errors.New("cpio: entry not found")
