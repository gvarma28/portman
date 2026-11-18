package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/gvarma28/portman/internal/comm"
	"github.com/gvarma28/portman/internal/ports"
	"github.com/gvarma28/portman/internal/sshpool"
	yaml "go.yaml.in/yaml/v3"
)

const sockPath = "/tmp/portman.sock"

var hostRegex = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func runDaemon(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath(), "path to the config file")
	fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := sshpool.New()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	pm := ports.NewPortMap(pool.Get)

	parseConfig(*configPath, pm)

	go sockServer(pm)

	<-ctx.Done()
	log.Println("shutting down gracefully")
}

func sockServer(pm *ports.PortMap) {
	// remove a stale socket file from a previous run
	os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	// clean up the socket file on Ctrl+C / SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		ln.Close()
		os.Remove(sockPath)
		os.Exit(0)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("accept:", err)
			return
		}
		go handle(conn, pm)
	}

}

func handle(conn net.Conn, pm *ports.PortMap) {
	defer conn.Close()
	for {
		raw, err := comm.ReadMsg(conn)
		if err != nil {
			return // io.EOF means the client disconnected
		}
		var cmd []string
		if err := json.Unmarshal(raw, &cmd); err != nil {
			log.Println("bad message:", err)
			return
		}
		log.Printf("received: %v\n", cmd)
		switch cmd[0] {
		case "add":
			if len(cmd[1:]) < 2 {
				err = fmt.Errorf("invalid args for 'add' operation, please provide host, port mapping")
				break
			}
			var host string
			host, err = parseHost(cmd[1])
			if err != nil {
				break
			}
			var local, remote int
			local, remote, err = parsePorts(cmd[2])
			if err != nil {
				break
			}
			pm.Add(local, remote, host)
			msg := fmt.Sprintf("added %s %d:%d", host, local, remote)
			if err = comm.SendResponse(conn, "success", msg, nil); err != nil {
				return
			}
		case "ls":
			p := pm.List()
			if err = comm.SendResponse(conn, "success", "host local:remote", p); err != nil {
				return
			}
		case "rm":
			if len(cmd[1:]) < 1 {
				err = fmt.Errorf("invalid args for 'rm' operation, please provide host, port mapping")
				break
			}
			success := []int{}
			for _, p := range cmd[1:] {
				local, err := parsePort(p)
				if err != nil {
					continue
				}

				pm.Remove(local)
				success = append(success, local)
			}
			msg := fmt.Sprintf("successully removed local ports: %v", success)
			if err = comm.SendResponse(conn, "success", msg, nil); err != nil {
				return
			}
		default:
			usage()
		}

		msg := fmt.Sprintf("operation failed: %v", err)
		if err := comm.SendResponse(conn, "error", msg, nil); err != nil {
			return
		}
	}
}

func parseHost(host string) (string, error) {
	host = strings.TrimSpace(host)

	name, ip, ok := strings.Cut(host, "@")
	if !ok {
		return "", fmt.Errorf("invalid host %q: is missing '@'", host)
	}
	if !hostRegex.MatchString(name) {
		return "", fmt.Errorf("invalid host %q: bad name %q", host, name)
	}
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid host %q: bad IP address %q", host, ip)
	}

	return host, nil
}

func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("%d out of range 1-65535", n)
	}
	return n, nil
}

func parsePorts(ports string) (int, int, error) {
	l, r, ok := strings.Cut(ports, ":")
	if !ok {
		return 0, 0, fmt.Errorf("invalid ports %q: expected local:remote", ports)
	}

	local, err := parsePort(l)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid local port: %w", err)
	}
	remote, err := parsePort(r)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid remote port: %w", err)
	}
	return local, remote, nil
}

// defaultConfigPath returns ~/.config/portman/config.yaml, or config.yaml in
// the working directory if the home directory can't be determined.
func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(home, ".config", "portman", "config.yaml")
}

func parseConfig(path string, portMap *ports.PortMap) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	var cfg ports.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatal(err)
	}

	for _, h := range cfg.Hosts {
		for _, f := range h.Forwards {
			err := portMap.Add(f.Local, f.Remote, h.Host)
			if err != nil {
				log.Printf("error while adding portMap: %v", err)

			}
		}
	}
}
