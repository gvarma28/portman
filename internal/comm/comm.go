package comm

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/gvarma28/portman/internal/ports"
)

const maxMsg = 16 << 20 // 16 MiB guard against bogus lengths

type Response struct {
	Status  string        `json:"status"`
	Message string        `json:"message"`
	Entries []ports.Entry `json:"entries"`
}

func WriteMsg(w io.Writer, payload []byte) error {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func ReadMsg(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxMsg {
		return nil, fmt.Errorf("message too large: %d", n)
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	return buf, err
}

func SendResponse(conn net.Conn, status, message string, entries []ports.Entry) error {
	resp, _ := json.Marshal(Response{
		Status:  status,
		Message: message,
		Entries: entries,
	})

	return WriteMsg(conn, resp)
}
