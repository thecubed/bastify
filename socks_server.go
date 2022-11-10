package main

import (
	"context"
	"fmt"
	"github.com/thecubed/go-socks5"
	"net"
)

const ctxKeyBastionHost = "BastionHost"
const ctxKeyBastionPort = "BastionPort"

type socksServer struct {
	Dial func(ctx context.Context, net, addr string) (net.Conn, error)
}

func (s *socksServer) Serve(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("Unable to listen: %q", err)
	}
	conf := &socks5.Config{
		Dial:     s.Dial,
		Rewriter: s,
	}

	conf.Credentials = alwaysValid{}

	server, err := socks5.New(conf)
	if err != nil {
		return fmt.Errorf("Unable to create SOCKS5 server: %v", err)
	}

	return server.Serve(l)
}

func (s *socksServer) Rewrite(ctx context.Context, request *socks5.Request) (context.Context, *socks5.AddrSpec) {
	ctx = context.WithValue(ctx, ctxKeyBastionHost, request.AuthContext.Payload["Username"])
	ctx = context.WithValue(ctx, ctxKeyBastionPort, request.AuthContext.Payload["Password"])
	return ctx, request.DestAddr
}

type alwaysValid struct{}

func (s alwaysValid) Valid(user, password string) bool {
	return true
}
