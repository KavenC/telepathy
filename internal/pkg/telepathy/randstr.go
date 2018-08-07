package telepathy

import (
	"math/rand"
	"time"
)

const seeds = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+-"
const (
	seedBits       = 6
	seedMask       = 1<<seedBits - 1
	seedSegmentMax = 63 / seedBits
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func getRandStr(length int) string {
	b := make([]byte, length)
	for currentByte, randbits, randbitsRemain := length-1, rand.Int63(), seedSegmentMax; currentByte >= 0; {
		if randbitsRemain == 0 {
			randbits, randbitsRemain = rand.Int63(), seedSegmentMax
		}
		idx := int(randbits & seedMask)
		b[currentByte] = seeds[idx]
		currentByte--
		randbits >>= seedBits
		randbitsRemain--
	}

	return string(b)
}
