package main

import (
	"context"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
	"net"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"
)

var opts struct {
	ListenHost     string        `short:"l" long:"listen-host" description:"SOCKS5 listen host" default:"127.0.0.1"`
	ListenPort     string        `short:"p" long:"listen-port" description:"SOCKS5 listen port." default:"5101"`
	SshUser        string        `short:"u" long:"user" description:"Bastion SSH username. Leave blank to use current user name." optional:"true"`
	SshKeyFile     string        `short:"k" long:"key-file" description:"Private key file to use when authenticating with bastion hosts. Leave unset to rely on SSH agent." optional:"true"`
	IdleClose      time.Duration `short:"t" long:"idle-close" description:"Idle timeout before closing bastion SSH connection." default:"4h"`
	Retries        int           `short:"r" long:"max-retries" description:"Maximum retries for a port forward through a bastion SSH connection" default:"2"`
	StatusInterval time.Duration `long:"status-interval" description:"Display connection statistics on this interval" default:"0"`
	Verbosity      []bool        `short:"v" description:"Change logging verbosity"`
}

type bastionHosts = map[string]*bastion

func statusPrinter(activeBastions bastionHosts) {
	hosts := make([]string, len(activeBastions))
	for k := range activeBastions {
		hosts = append(hosts, k)
	}
	log.WithFields(log.Fields{
		"known_bastions": len(activeBastions),
		"bastion_hosts":  strings.Join(hosts, ", "),
	}).Info("Tunnel status")
}

func main() {
	var mx sync.Mutex
	bastions := make(bastionHosts)

	// parse flags
	if _, err := flags.Parse(&opts); err != nil {
		switch flagsErr := err.(type) {
		case flags.ErrorType:
			if flagsErr == flags.ErrHelp {
				os.Exit(0)
			}
			os.Exit(1)
		default:
			os.Exit(1)
		}
	}

	// set up logger
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	verbosity := len(opts.Verbosity)
	if verbosity == 1 {
		log.SetLevel(log.DebugLevel)
	} else if verbosity >= 1 {
		log.SetLevel(log.TraceLevel)
	}

	// set up default ssh username from env if not provided on flags
	if opts.SshUser == "" {
		u, err := user.Current()
		if err != nil {
			log.Fatalf(err.Error())
		}
		opts.SshUser = u.Username
	}

	// set up a status printer
	if opts.StatusInterval > 0 {
		statusTimer := time.NewTicker(opts.StatusInterval)
		go func() {
			for {
				select {
				case _ = <-statusTimer.C:
					statusPrinter(bastions)
				}
			}
		}()
	}

	// set up socks listener
	s := socksServer{
		Dial: func(ctx context.Context, network, addr string) (conn net.Conn, err error) {
			// lock mutex to prevent concurrent creation of connections
			mx.Lock()

			// received a connection attempt, extract the username and password and use that to determine which
			// remote server should be used from the cache or dialled as a new connection
			bastionHost := ctx.Value(ctxKeyBastionHost).(string)
			bastionPort := ctx.Value(ctxKeyBastionPort).(string)
			lookupKey := bastionHost + ":" + bastionPort

			bastionLogger := log.WithField("bastion", lookupKey)
			bastionLogger.Tracef("Incoming SOCKS request")

			s, exists := bastions[lookupKey]
			if !exists {
				// no connection exists for bastion host, create a new one
				bastionLogger.Debug("Registering bastion host")
				s, err = NewBastion(net.JoinHostPort(bastionHost, bastionPort), opts.SshUser, opts.SshKeyFile, opts.Retries)
				if err != nil {
					mx.Unlock()
					bastionLogger.Errorf("Error creating bastion: %+v", err)
					return
				}
				bastions[lookupKey] = s
			}

			// unlock mutex
			mx.Unlock()

			// this should be switched to actually forward to the host from the address
			bastionLogger.Tracef("Forwarding connection")
			return s.ForwardTo(addr, opts.IdleClose)
		},
	}

	// serve forever
	addr := net.JoinHostPort(opts.ListenHost, opts.ListenPort)
	log.WithField("address", addr).Info("Serving SOCKS5 proxy")
	err := s.Serve(addr)
	if err != nil {
		panic(err)
	}
}
