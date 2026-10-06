package sshpool

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Pool struct {
	auth     ssh.AuthMethod
	hostKeys ssh.HostKeyCallback

	mu      sync.Mutex
	clients map[string]*ssh.Client // key: "user@host:port"
}

func New() (*Pool, error) {
	hostKeys, err := knownhosts.New(filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts"))
	if err != nil {
		return nil, err
	}
	sockPath := os.Getenv("SSH_AUTH_SOCK")
	if sockPath == "" {
		return nil, errors.New("SSH_AUTH_SOCK not set (is ssh-agent running?)")
	}
	sock, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("connect to ssh-agent: %w", err)
	}
	return &Pool{
		auth:     ssh.PublicKeysCallback(agent.NewClient(sock).Signers),
		hostKeys: hostKeys,
		clients:  make(map[string]*ssh.Client),
	}, nil
}

// Get returns a live client for target ("user@host" or "user@host:port")
// dialing only if there isn't one already.
func (p *Pool) Get(target string) (*ssh.Client, error) {
	user, addr, err := parseTarget(target)
	if err != nil {
		return nil, err
	}
	key := user + "@" + addr

	p.mu.Lock()
	defer p.mu.Unlock()

	if c, ok := p.clients[key]; ok {
		return c, nil
	}

	c, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{p.auth},
		HostKeyCallback: p.hostKeys,
		Timeout:         10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("ssh %s: %w", key, err)
	}
	p.clients[key] = c

	go func() {
		c.Wait()
		p.mu.Lock()
		if p.clients[key] == c {
			delete(p.clients, key)
		}
		p.mu.Unlock()
	}()

	return c, nil
}

// Close shuts down every client (for daemon shutdown).
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, c := range p.clients {
		c.Close()
		delete(p.clients, key)
	}
}

// parseTarget splits "user@host[:port]" into user and "host:port".
func parseTarget(target string) (user, addr string, err error) {
	user, host, ok := strings.Cut(target, "@")
	if !ok || user == "" || host == "" {
		return "", "", fmt.Errorf("invalid host %q: want user@host[:port]", target)
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		// No port given. Strip any IPv6 brackets ("[::1]") first, since
		// JoinHostPort adds its own.
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		host = net.JoinHostPort(host, "22")
	}
	return user, host, nil
}
