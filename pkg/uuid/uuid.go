package uuid

import (
	"crypto/md5"
	"crypto/rand"
	"fmt"
	"strings"
)

var dnsNamespace = [16]byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}

func NewV3FromName(name string) string {
	h := md5.New()
	h.Write(dnsNamespace[:])
	h.Write([]byte(strings.ToLower(name)))
	hash := h.Sum(nil)

	hash[6] = (hash[6] & 0x0f) | 0x30
	hash[8] = (hash[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
}

func NewV4() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
