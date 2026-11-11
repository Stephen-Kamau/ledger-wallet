package core

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"time"
)

// newID mints an opaque, roughly time-sortable identifier: a millisecond
// timestamp prefix followed by random bytes, hex-encoded. It is not a strict ULID
// but gives the same two properties core needs - uniqueness and lexical ordering
// that tracks creation time - without a dependency. A prefix tags the kind.
func newID(prefix string) string {
	var buf [16]byte
	binary.BigEndian.PutUint64(buf[:8], uint64(time.Now().UnixMilli()))
	_, _ = rand.Read(buf[8:])
	return prefix + "_" + hex.EncodeToString(buf[:])
}
