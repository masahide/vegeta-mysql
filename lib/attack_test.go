package vegeta

import (
	"io"
	"testing"
	"time"
)

func TestBadTargeterError(t *testing.T) {
	atk := NewAttacker()
	tr := func(*Target) error { return io.EOF }
	res := atk.hit(tr, time.Now())
	if got, want := res.Error, io.EOF.Error(); got != want {
		t.Fatalf("got: %v, want: %v", got, want)
	}
}
