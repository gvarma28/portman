package ports

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type Dialer func(host string) (*ssh.Client, error)

// Entry is the public, serializable view of one forward (for `portman ls`).
type Entry struct {
	Local  int    `json:"local" yaml:"local"`
	Remote int    `json:"remote" yaml:"remote"`
	Host   string `json:"host" yaml:"host"`
}

type mapping struct {
	Entry
	ln     net.Listener
	closed chan struct{} // closed on Remove; tells active connections to stop
}

type PortMap struct {
	dial Dialer

	mu    sync.Mutex
	ports map[int]*mapping
}

func NewPortMap(dial Dialer) *PortMap {
	return &PortMap{dial: dial, ports: make(map[int]*mapping)}
}

func (p *PortMap) Add(local, remote int, host string) error {
	if !validPort(local) || !validPort(remote) {
		return fmt.Errorf("invalid ports: local:%d remote:%d", local, remote)
	}

	// Fail early on a bad host. Done before taking the lock, because
	// dialing can take seconds and would block List/Remove meanwhile.
	if _, err := p.dial(host); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if m, ok := p.ports[local]; ok {
		return fmt.Errorf("local port %d is already forwarded to %s:%d", local, m.Host, m.Remote)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", local))
	if err != nil {
		return err
	}
	m := &mapping{
		Entry:  Entry{Local: local, Remote: remote, Host: host},
		ln:     ln,
		closed: make(chan struct{}),
	}
	p.ports[local] = m
	go p.serve(m)

	log.Printf("forwarding localhost:%d -> %s localhost:%d", local, host, remote)
	return nil
}

func (p *PortMap) Remove(local int) error {
	p.mu.Lock()
	m, ok := p.ports[local]
	if ok {
		delete(p.ports, local)
	}
	p.mu.Unlock()

	if !ok {
		return fmt.Errorf("local port %d is not forwarded", local)
	}
	close(m.closed)     // active handle() calls exit
	return m.ln.Close() // accept loop exits via net.ErrClosed
}

// List returns a snapshot of all forwards, sorted by local port.
func (p *PortMap) List() []Entry {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]Entry, 0, len(p.ports))
	for _, m := range p.ports {
		out = append(out, m.Entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Local < out[j].Local })
	return out
}

func (p *PortMap) serve(m *mapping) {
	for {
		conn, err := m.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("accept on :%d: %v", m.Local, err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		go p.handle(m, conn)
	}
}

func (p *PortMap) handle(m *mapping, local net.Conn) {
	defer local.Close()

	client, err := p.dial(m.Host) // per connection: picks up reconnects
	if err != nil {
		log.Printf("ssh %s: %v", m.Host, err)
		return
	}
	remote, err := client.Dial("tcp", fmt.Sprintf("localhost:%d", m.Remote))
	if err != nil {
		log.Printf("remote dial %s localhost:%d: %v", m.Host, m.Remote, err)
		return
	}
	defer remote.Close()

	done := make(chan struct{}, 2)
	go pipe(remote, local, done)
	go pipe(local, remote, done)

	// Wait for both directions to finish, or for the forward to be removed.
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-m.closed:
			return
		}
	}
}

// pipe copies src to dst, then half-closes dst so the other side sees EOF
// but can still send its reply.
func pipe(dst, src net.Conn, done chan<- struct{}) {
	io.Copy(dst, src)
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	} else {
		dst.Close()
	}
	done <- struct{}{}
}

func validPort(n int) bool { return n > 0 && n <= 65535 }
