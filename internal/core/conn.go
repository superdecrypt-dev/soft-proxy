package core

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

type PeakableConn struct {
	net.Conn
	peekBuf []byte
	offset  int
	mu      sync.Mutex
}

func NewPeakableConn(c net.Conn) *PeakableConn {
	return &PeakableConn{Conn: c}
}

func (c *PeakableConn) Peek(n int) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	needed := n - len(c.peekBuf)
	if needed > 0 {
		buf := make([]byte, needed)
		_ = c.Conn.SetReadDeadline(time.Now().Add(3 * time.Second))

		rn, err := c.Conn.Read(buf)
		_ = c.Conn.SetReadDeadline(time.Time{})

		if rn > 0 {
			c.peekBuf = append(c.peekBuf, buf[:rn]...)
		}
		if err != nil && (!errors.Is(err, io.EOF) || len(c.peekBuf) == 0) {
			return nil, err
		}
	}

	if len(c.peekBuf) < n {
		return c.peekBuf, nil
	}
	return c.peekBuf[:n], nil
}

func (c *PeakableConn) PeekTLSRecord() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for len(c.peekBuf) < 5 {
		buf := make([]byte, 5-len(c.peekBuf))
		_ = c.Conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := c.Conn.Read(buf)
		_ = c.Conn.SetReadDeadline(time.Time{})
		if n > 0 {
			c.peekBuf = append(c.peekBuf, buf[:n]...)
		}
		if err != nil {
			return nil, err
		}
	}

	if c.peekBuf[0] != 0x16 {
		return c.peekBuf, nil
	}

	recordLen := int(c.peekBuf[3])<<8 | int(c.peekBuf[4])
	if recordLen > 16384 {
		recordLen = 16384
	}
	totalLen := 5 + recordLen

	for len(c.peekBuf) < totalLen {
		buf := make([]byte, totalLen-len(c.peekBuf))
		_ = c.Conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := c.Conn.Read(buf)
		_ = c.Conn.SetReadDeadline(time.Time{})
		if n > 0 {
			c.peekBuf = append(c.peekBuf, buf[:n]...)
		}
		if err != nil {
			return nil, err
		}
	}

	return c.peekBuf[:totalLen], nil
}

func (c *PeakableConn) Read(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.offset < len(c.peekBuf) {
		n := copy(b, c.peekBuf[c.offset:])
		c.offset += n
		return n, nil
	}

	return c.Conn.Read(b)
}

type ChanListener struct {
	connChan chan net.Conn
	addr     net.Addr
}

func (l *ChanListener) Accept() (net.Conn, error) {
	conn, ok := <-l.connChan
	if !ok {
		return nil, errors.New("listener closed")
	}
	return conn, nil
}

func (l *ChanListener) Close() error {
	close(l.connChan)
	return nil
}

func (l *ChanListener) Addr() net.Addr {
	return l.addr
}

type sniSniffConn struct {
	net.Conn
	r io.Reader
}

func (c *sniSniffConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}

func (c *sniSniffConn) Write(b []byte) (int, error) {
	return len(b), nil
}

func DetectProtocol(data []byte) string {
	if len(data) == 0 {
		return "http"
	}

	if data[0] == 0x00 {
		return "vless"
	}

	if len(data) >= 58 && data[56] == '\r' && data[57] == '\n' {
		isHex := true
		for i := 0; i < 56; i++ {
			c := data[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				isHex = false
				break
			}
		}
		if isHex {
			return "trojan"
		}
	}

	strData := string(data)
	httpMethods := []string{"GET ", "POST ", "PUT ", "DELETE ", "OPTIONS ", "HEAD ", "CONNECT ", "PATCH ", "PRI "}
	for _, method := range httpMethods {
		if strings.HasPrefix(strData, method) {
			return "http"
		}
	}

	return "vmess"
}

func ParseHTTPPath(data []byte) string {
	lineEnd := bytes.Index(data, []byte("\r\n"))
	if lineEnd == -1 {
		lineEnd = bytes.Index(data, []byte("\n"))
	}
	if lineEnd == -1 {
	return "vmess"
}
	firstLine := data[:lineEnd]
	parts := bytes.Split(firstLine, []byte(" "))
	if len(parts) >= 2 {
		return string(parts[1])
	}
	return ""
}
