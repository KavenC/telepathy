package randstr

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	for round := 100; round > 0; round-- {
		out := Generate(5)
		if len(out) != 5 {
			t.Logf("Result length error")
			t.Fail()
		}

		for _, r := range out {
			ch := string(r)
			if !strings.Contains(seeds, ch) {
				t.Logf("Result contains wrong character: %s", ch)
				t.Fail()
			}
		}
	}
}
