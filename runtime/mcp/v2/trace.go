package mcpv2

import "crypto/rand"

func GenerateTraceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hexEncode(b)
}

func GenerateSpanID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hexEncode(b)
}

func hexEncode(b []byte) string {
	const hex = "0123456789abcdef"
	res := make([]byte, len(b)*2)
	for i, v := range b {
		res[i*2] = hex[v>>4]
		res[i*2+1] = hex[v&0x0f]
	}
	return string(res)
}
