package core

// TestSecondGen writes a log with seal parts (a chunk of 3) for the second
// verifier in tools/second. It runs only when BEAN_SECOND_OUT is set.
import (
	"os"
	"testing"
)

func TestSecondGen(t *testing.T) {
	out := os.Getenv("BEAN_SECOND_OUT")
	if out == "" {
		t.Skip()
	}
	withChunk(t, 3)
	s := Open(out + "/parts")
	_, words, err := s.Init(NewKey{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 11; i++ {
		appendLines(t, s, partial("hr", float64(i), nil))
	}
	if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
	appendLines(t, s, partial("hr", 99, nil))
	if _, err := s.Rotate(words, Seals{All: true}, NewKey{}); err != nil {
		t.Fatal(err)
	}
}
