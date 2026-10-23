package backend

import (
	"bytes"
	"encoding/binary"
)

var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// icnsPNG returns the best PNG inside a macOS .icns file: the largest of
// the PNG-encoded sizes up to 512 px, which modern icons all carry.
func icnsPNG(b []byte) []byte {
	if len(b) < 8 || string(b[:4]) != "icns" {
		return nil
	}
	// Preference by type: 256, 512, 128 px, then their @2x variants.
	rank := map[string]int{"ic08": 6, "ic09": 5, "ic13": 4, "ic07": 3, "ic14": 2, "ic12": 1}
	var best []byte
	bestRank := 0
	for off := 8; off+8 <= len(b); {
		typ := string(b[off : off+4])
		size := int(binary.BigEndian.Uint32(b[off+4 : off+8]))
		if size < 8 || off+size > len(b) {
			break
		}
		data := b[off+8 : off+size]
		if r := rank[typ]; r > bestRank && bytes.HasPrefix(data, pngMagic) {
			best, bestRank = data, r
		}
		off += size
	}
	return best
}
