package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	vegeta "github.com/masahide/vegeta/lib"
)

func attackCmd() command {
	fs := flag.NewFlagSet("vegeta attack", flag.ExitOnError)
	opts := &attackOpts{}

	fs.StringVar(&opts.targetsf, "targets", "stdin", "Targets file")
	fs.StringVar(&opts.outputf, "output", "stdout", "Output file")
	fs.StringVar(&opts.bodyf, "body", "", "Requests body file")
	fs.BoolVar(&opts.lazy, "lazy", false, "Read targets lazily")
	fs.DurationVar(&opts.duration, "duration", 10*time.Second, "Duration of the test")
	fs.DurationVar(&opts.timeout, "timeout", vegeta.DefaultTimeout, "Requests timeout")
	fs.Uint64Var(&opts.rate, "rate", 50, "Requests per second")
	fs.Uint64Var(&opts.workers, "workers", vegeta.DefaultWorkers, "Initial number of workers")
	fs.IntVar(&opts.maxOpenConns, "maxOpenConns", vegeta.DefaultConnections, "Max open connections per target host")
	fs.IntVar(&opts.maxIdleConns, "maxIdleConns", vegeta.DefaultConnections, "Max open idle connections per target host")
	fs.StringVar(&opts.dsn, "dsn", "password@protocol(address)/dbname?param=value", "Data Source Name has a common format")

	return command{fs, func(args []string) error {
		fs.Parse(args)
		return attack(opts)
	}}
}

var (
	errZeroDuration = errors.New("duration must be bigger than zero")
	errZeroRate     = errors.New("rate must be bigger than zero")
	errBadCert      = errors.New("bad certificate")
)

// attackOpts aggregates the attack function command options
type attackOpts struct {
	targetsf     string
	outputf      string
	bodyf        string
	lazy         bool
	duration     time.Duration
	timeout      time.Duration
	rate         uint64
	workers      uint64
	maxOpenConns int
	maxIdleConns int
	dsn          string
}

// attack validates the attack arguments, sets up the
// required resources, launches the attack and writes the results
func attack(opts *attackOpts) (err error) {
	if opts.rate == 0 {
		return errZeroRate
	}

	if opts.duration == 0 {
		return errZeroDuration
	}

	files := map[string]io.Reader{}
	for _, filename := range []string{opts.targetsf, opts.bodyf} {
		if filename == "" {
			continue
		}
		f, err := file(filename, false)
		if err != nil {
			return fmt.Errorf("error opening %s: %s", filename, err)
		}
		defer f.Close()
		files[filename] = f
	}

	var body []byte
	if bodyf, ok := files[opts.bodyf]; ok {
		if body, err = ioutil.ReadAll(bodyf); err != nil {
			return fmt.Errorf("error reading %s: %s", opts.bodyf, err)
		}
	}

	var (
		tr  vegeta.Targeter
		src = files[opts.targetsf]
		//hdr = opts.headers.Header
	)
	if opts.lazy {
		tr = vegeta.NewLazyTargeter(src, body)
	} else if tr, err = vegeta.NewEagerTargeter(src, body); err != nil {
		return err
	}

	out, err := file(opts.outputf, true)
	if err != nil {
		return fmt.Errorf("error opening %s: %s", opts.outputf, err)
	}
	defer out.Close()

	atk := vegeta.NewAttacker(
		vegeta.Workers(opts.workers),
		vegeta.Dsn(opts.dsn),
		vegeta.SetMaxIdleConns(opts.maxIdleConns),
		vegeta.SetMaxOpenConns(opts.maxOpenConns),
	)

	res := atk.Attack(tr, opts.rate, opts.duration)
	enc := vegeta.NewEncoder(out)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	for {
		select {
		case <-sig:
			atk.Stop()
			return nil
		case r, ok := <-res:
			if !ok {
				return nil
			}
			if err = enc.Encode(r); err != nil {
				return err
			}
		}
	}
}

// headers is the http.Header used in each target request
// it is defined here to implement the flag.Value interface
// in order to support multiple identical flags for request header
// specification
type headers struct{ http.Header }

func (h headers) String() string {
	buf := &bytes.Buffer{}
	if err := h.Write(buf); err != nil {
		return ""
	}
	return buf.String()
}

// Set implements the flag.Value interface for a map of HTTP Headers.
func (h headers) Set(value string) error {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("header '%s' has a wrong format", value)
	}
	key, val := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if key == "" || val == "" {
		return fmt.Errorf("header '%s' has a wrong format", value)
	}
	// Add key/value directly to the http.Header (map[string][]string).
	// http.Header.Add() cannonicalizes keys but vegeta is used
	// to test systems that require case-sensitive headers.
	h.Header[key] = append(h.Header[key], val)
	return nil
}

// localAddr implements the Flag interface for parsing net.IPAddr
type localAddr struct{ *net.IPAddr }

func (ip *localAddr) Set(value string) (err error) {
	ip.IPAddr, err = net.ResolveIPAddr("ip", value)
	return
}
