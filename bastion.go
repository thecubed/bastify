package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"io/ioutil"
	"net"
	"os"
	"sync/atomic"
	"time"
)

type bastion struct {
	config     *ssh.ClientConfig
	addr       string
	client     atomic.Value // *ssh.Client
	closeTimer *time.Timer
	retries    int
	logger     *log.Entry
	Active     bool
}

func NewBastion(addr string, user string, keyFile string, retries int) (*bastion, error) {
	// set up bastion logger
	logger := log.WithField("bastion", addr)

	// set up ssh agent
	socket := os.Getenv("SSH_AUTH_SOCK")
	conn, err := net.Dial("unix", socket)
	if err != nil {
		logger.Errorf("Failed to open SSH_AUTH_SOCK: %s", err)
		return nil, err
	}
	agentClient := agent.NewClient(conn)

	authMethods := []ssh.AuthMethod{
		ssh.PublicKeysCallback(agentClient.Signers),
	}

	if keyFile != "" {
		key, err := ioutil.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("open ssh key %v failed: %w", keyFile, err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse ssh key failed: %w", err)
		}

		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	logger.Debugf("New bastion server registered")

	return &bastion{config: config, addr: addr, closeTimer: time.NewTimer(0), retries: retries, logger: logger, Active: false}, nil
}

func (s *bastion) ForwardTo(addr string, sshIdleClose time.Duration) (conn net.Conn, err error) {
	if !s.closeTimer.Stop() {
		// We do not drain the channel here to avoid race condition with the
		// goroutine created below. This branch is very unlikely to be hit in
		// reality though.
	}
	s.closeTimer.Reset(sshIdleClose)
	var redialSSH bool
	for i := 0; i < s.retries; i++ {
		// fetch atomic value and cast it back to a ssh client type
		client, ok := s.client.Load().(*ssh.Client)

		if !ok || redialSSH {
			// no ssh client, or redial flag set
			s.logger.Debug("Dialling SSH connection to bastion")
			client, err = ssh.Dial("tcp", s.addr, s.config)
			if err != nil {
				return nil, err
			}

			// set up idle connection close timer
			go func() {
				_ = <-s.closeTimer.C
				s.logger.Debug("Connection idle, closing SSH connection to bastion")
				client.Close()
				s.Active = false
				// TODO: this is a known memory leak, closing a connection to a bastion server does not remove it
				// from the connections list in main.go. This memory leak would only be problematic if you connect
				// to a large number of bastion servers over time, as the list will accumulate every bastion ever seen
			}()

			// store the ssh client in the atomic value
			s.client.Store(client)
			s.Active = true
		}

		// create the tunnelled connection via the client
		s.logger.WithField("destination", addr).Debugf("Dialling tunnelled connection to destination host")
		// TODO: handle DNS resolution on the server side
		conn, err = client.Dial("tcp", addr)
		if err != nil {
			// could not tunnel connection through bastion, close the bastion connection and retry
			s.logger.Errorf("Error from ssh client dial. Will retry %d more times: %s", s.retries-i, err)
			redialSSH = true
			continue
		}
		return conn, nil
	}
	return nil, err
}
