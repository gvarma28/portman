package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v3"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

func sshConfig(user string) (*ssh.ClientConfig, error) {
	hostKeys, err := knownhosts.New(filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts"))
	if err != nil {
		return nil, err
	}
	sockPath := os.Getenv("SSH_AUTH_SOCK")
	if sockPath == "" {
		return nil, errors.New("SSH_AUTH_SOCK not set (is ssh-running?)")
	}
	sock, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("connect to ssh-agent: %w", err)
	}
	return &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeysCallback(agent.NewClient(sock).Signers)},
		HostKeyCallback: hostKeys,
		Timeout:         10 * time.Second,
	}, nil
}

func main() {
	cfg := parseConfig("config.yaml")

	for _, h := range cfg.Hosts {
		user, addr, ok := strings.Cut(h.Host, "@")
		if !ok {
			log.Fatal("usage: invalid host: ", h.Host)
		}
		cfg, err := sshConfig(user)
		if err != nil {
			log.Fatal(err)
		}
		if !strings.Contains(addr, ":") {
			addr += ":22"
		}

		client, err := ssh.Dial("tcp", addr, cfg)
		if err != nil {
			log.Fatal("ssh: ", err)
		}
		defer client.Close()
		for _, p := range h.Forwards {
			if p.Local == 0 || p.Remote == 0 {
				log.Fatalf("usage: invalid ports: local:%d, remote:%d", p.Local, p.Remote)
			}

			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p.Local))
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("forwarding localhost:%d -> %s localhost:%d", p.Local, addr, p.Remote)

			go func(ln net.Listener, client *ssh.Client, remote int) {
				for {
					conn, err := ln.Accept()
					if err != nil {
						if errors.Is(err, net.ErrClosed) {
							return
						}
						time.Sleep(100 * time.Millisecond)
						log.Println("accept:", err)
						continue
					}
					log.Println("handling loop")
					go handle(client, conn, remote)
				}
			}(ln, client, p.Remote)

		}
	}

	select {} // block forever; the accept goroutines do the work
}

func parseConfig(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatal(err)
	}

	return cfg
}

func handle(client *ssh.Client, local net.Conn, port int) {
	defer local.Close()

	remote, err := client.Dial("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Printf("remote dial: %v", err)
		return
	}
	defer remote.Close()

	done := make(chan struct{}, 2)
	go func() { io.Copy(remote, local); done <- struct{}{} }()
	go func() { io.Copy(local, remote); done <- struct{}{} }()
	<-done // when either side finishes, close both
}
