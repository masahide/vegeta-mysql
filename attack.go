package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	vegeta "github.com/masahide/vegeta-mysql/lib"
)

func attackCmd() command {
	fs := flag.NewFlagSet("vegeta attack", flag.ExitOnError)
	opts := &attackOpts{}

	fs.StringVar(&opts.outputf, "output", "stdout", "Output file")
	fs.StringVar(&opts.bodyf, "body", "", "Requests body file")
	fs.DurationVar(&opts.duration, "duration", 10*time.Second, "Duration of the test")
	fs.DurationVar(&opts.timeout, "timeout", vegeta.DefaultTimeout, "Requests timeout")
	fs.Uint64Var(&opts.rate, "rate", 50, "Requests per second")
	fs.Uint64Var(&opts.workers, "workers", vegeta.DefaultWorkers, "Initial number of workers")
	fs.IntVar(&opts.maxOpenConns, "maxOpenConns", vegeta.DefaultConnections, "Max open connections per target host")
	fs.IntVar(&opts.maxIdleConns, "maxIdleConns", vegeta.DefaultConnections, "Max open idle connections per target host")
	fs.BoolVar(&opts.Persistent, "pconnect", vegeta.DefaultPersistent, "Persistent connection")
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
	outputf      string
	bodyf        string
	duration     time.Duration
	timeout      time.Duration
	rate         uint64
	workers      uint64
	maxOpenConns int
	maxIdleConns int
	Persistent   bool
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

	filename := opts.bodyf
	f, err := file(filename, false)
	if err != nil {
		return fmt.Errorf("error opening %s: %s", filename, err)
	}
	defer f.Close()

	var (
		tr vegeta.Targeter
	)
	if tr, err = vegeta.NewEagerTargeter(f); err != nil {
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
		vegeta.SetPersistent(opts.Persistent),
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

// localAddr implements the Flag interface for parsing net.IPAddr
type localAddr struct{ *net.IPAddr }

func (ip *localAddr) Set(value string) (err error) {
	ip.IPAddr, err = net.ResolveIPAddr("ip", value)
	return
}
