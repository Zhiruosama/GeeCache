package geecache

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"geecache/geecache/consistenthash"
	"io"
	"log"
	"net"
	"sync"
)

const defaultReplicas = 50

// Response status codes
const (
	statusOK            = 0
	statusNotFound      = 1
	statusInternalError = 2
	statusBadRequest    = 3
)

// TCPPool implements PeerPicker for a pool of TCP peers.
type TCPPool struct {
	self       string // e.g. "localhost:8001"
	logEnabled bool
	mu         sync.RWMutex
	peers      *consistenthash.Map
	getters    map[string]*tcpGetter
}

type tcpGetter struct {
	addr string
	pool *connPool
}

type connPool struct {
	addr    string
	mu      sync.Mutex
	idle    []net.Conn
	count   int
	maxConn int
}

func NewTCPPool(self string) *TCPPool {
	return &TCPPool{self: self}
}

func (p *TCPPool) SetLog(enabled bool) {
	p.logEnabled = enabled
}

func (p *TCPPool) Log(format string, v ...interface{}) {
	if !p.logEnabled {
		return
	}
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}

func (p *TCPPool) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = consistenthash.New(defaultReplicas, nil)
	p.peers.Add(peers...)
	p.getters = make(map[string]*tcpGetter, len(peers))
	for _, peer := range peers {
		p.getters[peer] = &tcpGetter{
			addr: peer,
			pool: &connPool{addr: peer, maxConn: 64},
		}
	}
}

func (p *TCPPool) PickPeer(key string) (PeerGetter, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		p.Log("Pick peer %s", peer)
		return p.getters[peer], true
	}
	return nil, false
}

// --- connection pool ---

func (cp *connPool) get() (net.Conn, error) {
	cp.mu.Lock()
	if n := len(cp.idle); n > 0 {
		conn := cp.idle[n-1]
		cp.idle = cp.idle[:n-1]
		cp.mu.Unlock()
		return conn, nil
	}
	cp.count++
	cp.mu.Unlock()
	return net.Dial("tcp", cp.addr)
}

func (cp *connPool) put(conn net.Conn) {
	cp.mu.Lock()
	if len(cp.idle) < cp.maxConn {
		cp.idle = append(cp.idle, conn)
		cp.mu.Unlock()
		return
	}
	cp.count--
	cp.mu.Unlock()
	conn.Close()
}

func (cp *connPool) discard(conn net.Conn) {
	cp.mu.Lock()
	cp.count--
	cp.mu.Unlock()
	conn.Close()
}

// --- client (tcpGetter) ---

func (g *tcpGetter) Get(group string, key string) ([]byte, error) {
	conn, err := g.pool.get()
	if err != nil {
		return nil, err
	}

	bw := bufio.NewWriter(conn)
	// write request: [uint16 groupLen][uint16 keyLen][group][key]
	var hdr [4]byte
	binary.BigEndian.PutUint16(hdr[0:2], uint16(len(group)))
	binary.BigEndian.PutUint16(hdr[2:4], uint16(len(key)))
	bw.Write(hdr[:])
	bw.WriteString(group)
	bw.WriteString(key)
	if err := bw.Flush(); err != nil {
		g.pool.discard(conn)
		return nil, err
	}

	br := bufio.NewReader(conn)
	// read response: [uint8 status][uint32 bodyLen][body]
	var respHdr [5]byte
	if _, err := io.ReadFull(br, respHdr[:]); err != nil {
		g.pool.discard(conn)
		return nil, err
	}
	status := respHdr[0]
	bodyLen := binary.BigEndian.Uint32(respHdr[1:5])

	body := make([]byte, bodyLen)
	if bodyLen > 0 {
		if _, err := io.ReadFull(br, body); err != nil {
			g.pool.discard(conn)
			return nil, err
		}
	}

	g.pool.put(conn)

	if status != statusOK {
		return nil, fmt.Errorf("peer %s: %s", g.addr, string(body))
	}
	return body, nil
}

// --- server ---

func (p *TCPPool) ListenAndServe() error {
	ln, err := net.Listen("tcp", p.self)
	if err != nil {
		return err
	}
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[TCPPool] accept error: %v", err)
			continue
		}
		go p.handleConn(conn)
	}
}

func (p *TCPPool) handleConn(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	bw := bufio.NewWriter(conn)

	for {
		// read request header
		var hdr [4]byte
		if _, err := io.ReadFull(br, hdr[:]); err != nil {
			return // EOF or connection closed
		}
		groupLen := binary.BigEndian.Uint16(hdr[0:2])
		keyLen := binary.BigEndian.Uint16(hdr[2:4])

		buf := make([]byte, int(groupLen)+int(keyLen))
		if _, err := io.ReadFull(br, buf); err != nil {
			return
		}
		groupName := string(buf[:groupLen])
		key := string(buf[groupLen:])

		group := GetGroup(groupName)
		if group == nil {
			writeResponse(bw, statusNotFound, []byte("no such group: "+groupName))
			continue
		}

		view, err := group.Get(key)
		if err != nil {
			writeResponse(bw, statusInternalError, []byte(err.Error()))
			continue
		}

		writeResponse(bw, statusOK, view.Bytes())
	}
}

func writeResponse(bw *bufio.Writer, status uint8, body []byte) {
	var hdr [5]byte
	hdr[0] = status
	binary.BigEndian.PutUint32(hdr[1:5], uint32(len(body)))
	bw.Write(hdr[:])
	bw.Write(body)
	bw.Flush()
}

var _ PeerPicker = (*TCPPool)(nil)
var _ PeerGetter = (*tcpGetter)(nil)
