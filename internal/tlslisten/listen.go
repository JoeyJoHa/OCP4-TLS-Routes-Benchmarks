package tlslisten

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	appconfig "github.com/JoeyJoHa/OCP4-TLS-Routes-Benchmarks/internal/config"
)

type ctxKey struct{}

const handshakeTimeout = 10 * time.Second

// Meta is per-connection TLS handshake timing.
type Meta struct {
	Handshake time.Duration
	requests  atomic.Uint64
}

// ConsumeHandshake returns handshake milliseconds on the first request
// of a connection, and reused=true for keep-alive follow-ups.
func (m *Meta) ConsumeHandshake() (handshakeMs float64, reused bool) {
	if m == nil {
		return 0, false
	}
	n := m.requests.Add(1)
	if n > 1 {
		return 0, true
	}
	return float64(m.Handshake.Microseconds()) / 1000.0, false
}

// Tracker stores handshake metadata keyed by the accepted connection.
type Tracker struct {
	byConn sync.Map
}

func NewTracker() *Tracker {
	return &Tracker{}
}

func (t *Tracker) ConnContext(ctx context.Context, conn net.Conn) context.Context {
	if t == nil {
		return ctx
	}
	value, ok := t.byConn.Load(conn)
	if !ok {
		return ctx
	}
	meta, ok := value.(*Meta)
	if !ok {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, meta)
}

func (t *Tracker) ConnState(conn net.Conn, state http.ConnState) {
	if t == nil {
		return
	}
	if state == http.StateClosed || state == http.StateHijacked {
		t.byConn.Delete(conn)
	}
}

func FromContext(ctx context.Context) *Meta {
	meta, _ := ctx.Value(ctxKey{}).(*Meta)
	return meta
}

type listener struct {
	net.Listener
	config  *tls.Config
	tracker *Tracker
}

// New wraps a TCP listener so Accept completes the TLS handshake and
// records its duration. The returned connection is *tls.Conn so HTTP/2 works.
func New(inner net.Listener, config *tls.Config, tracker *Tracker) net.Listener {
	if config == nil {
		config = &tls.Config{}
	}
	if len(config.NextProtos) == 0 {
		cloned := config.Clone()
		cloned.NextProtos = append([]string(nil), appconfig.DefaultTLSNextProtos...)
		config = cloned
	}
	return &listener{Listener: inner, config: config, tracker: tracker}
}

func (l *listener) Accept() (net.Conn, error) {
	for {
		raw, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		tlsConn := tls.Server(raw, l.config)
		started := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
		err = tlsConn.HandshakeContext(ctx)
		cancel()
		if err != nil {
			log.Printf("tls handshake: %v", err)
			if closeErr := tlsConn.Close(); closeErr != nil {
				log.Printf("close failed handshake: %v", closeErr)
			}
			continue
		}
		meta := &Meta{Handshake: time.Since(started)}
		if l.tracker != nil {
			l.tracker.byConn.Store(tlsConn, meta)
		}
		return tlsConn, nil
	}
}
