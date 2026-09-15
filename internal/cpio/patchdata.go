package cpio

import "bytes"

// Verity and encryption options stripped from fstab entries when the caller
// asks not to keep dm-verity or forced encryption. Ported from Magisk's
// native/src/boot/patch.rs.

var verityPatterns = [][]byte{
	[]byte("verifyatboot"),
	[]byte("verify"),
	[]byte("avb_keys"),
	[]byte("avb"),
	[]byte("support_scfs"),
	[]byte("fsverity"),
}

var encryptionPatterns = [][]byte{
	[]byte("forceencrypt"),
	[]byte("forcefdeorfbe"),
	[]byte("fileencryption"),
}

// matchPattern returns the number of bytes to skip when buf starts with one of
// patterns, or 0 when nothing matches.
//
// A match may include a leading comma (so that ",verify" is removed together
// with its separator) and, when the option carries a value, the "=value" part
// up to the next space, newline, comma or NUL.
func matchPattern(buf []byte, patterns [][]byte) int {
	if len(buf) == 0 {
		return 0
	}
	n := 0
	if buf[0] == ',' {
		n = 1
	}
	if n >= len(buf) {
		return 0
	}
	rest := buf[n:]
	matched := false
	for _, p := range patterns {
		if bytes.HasPrefix(rest, p) {
			n += len(p)
			matched = true
			break
		}
	}
	if !matched {
		return 0
	}
	if rest = buf[n:]; len(rest) > 0 && rest[0] == '=' {
		for _, c := range rest {
			if c == ' ' || c == '\n' || c == ',' || c == 0 {
				break
			}
			n++
		}
	}
	return n
}

// removePattern deletes every occurrence of the given patterns from buf and
// returns the length of the compacted data. The tail of buf is zero filled,
// matching magiskboot's in-place behaviour.
func removePattern(buf []byte, patterns [][]byte) int {
	write, read, size := 0, 0, len(buf)
	for read < len(buf) {
		if n := matchPattern(buf[read:], patterns); n > 0 {
			size -= n
			read += n
			continue
		}
		buf[write] = buf[read]
		write++
		read++
	}
	for i := write; i < len(buf); i++ {
		buf[i] = 0
	}
	return size
}

// PatchVerity strips dm-verity and AVB options from an fstab blob.
func PatchVerity(buf []byte) int { return removePattern(buf, verityPatterns) }

// PatchEncryption strips forced-encryption options from an fstab blob.
func PatchEncryption(buf []byte) int { return removePattern(buf, encryptionPatterns) }
