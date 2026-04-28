package importeos

import (
	"encoding/hex"
	"unicode/utf16"
)

// decodeUText decodes a $$UText field list into a UTF-8 string.
// Encoding: each token is hex digits; concatenated bytes are UTF-16-LE.
// Returns false if decoding fails.
func decodeUText(fields []string) (string, bool) {
	var raw []byte
	for _, f := range fields {
		b, err := hex.DecodeString(f)
		if err != nil {
			return "", false
		}
		raw = append(raw, b...)
	}
	if len(raw)%2 != 0 {
		return "", false
	}
	u16 := make([]uint16, len(raw)/2)
	for i := range u16 {
		u16[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	return string(utf16.Decode(u16)), true
}
