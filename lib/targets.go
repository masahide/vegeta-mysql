package vegeta

import (
	"bufio"
	"errors"
	"os"
)

// Target is an HTTP request blueprint.
type Target struct {
	Body string
}

// Request creates an *http.Request out of Target and returns it along with an
// error in case of failure.
func (t *Target) Query() (string, error) {
	return t.Body, nil
}

var (
	// ErrNoTargets is returned when not enough Targets are available.
	ErrNoTargets = errors.New("no targets to attack")
	// ErrNilTarget is returned when the passed Target pointer is nil.
	ErrNilTarget = errors.New("nil target")
)

// A Targeter decodes a Target or returns an error in case of failure.
// Implementations must be safe for concurrent use.
type Targeter func(*Target) error

// NewStaticTargeter returns a Targeter which round-robins over the passed
// Targets.
func NewStaticTargeter(inCh chan string) Targeter {
	return func(tgt *Target) error {
		*tgt = Target{Body: <-inCh}
		return nil
	}
}

// NewEagerTargeter eagerly reads all Targets out of the provided io.Reader and
// returns a NewStaticTargeter with them.
//
// body will be set as the Target's body if no body is provided.
// hdr will be merged with the each Target's headers.
func NewEagerTargeter(f *os.File) (Targeter, error) {
	inCh := make(chan string, 100)
	go func(target chan string, in *os.File) {
		for {
			scanner := bufio.NewScanner(in)
			for scanner.Scan() {
				target <- scanner.Text()
			}
			in.Seek(0, 0)
		}
	}(inCh, f)
	return NewStaticTargeter(inCh), nil
}
