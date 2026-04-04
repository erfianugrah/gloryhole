package unbound

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	dnstap "github.com/dnstap/golang-dnstap"
	framestream "github.com/farsightsec/golang-framestream"
	"github.com/miekg/dns"
	"glory-hole/pkg/logging"
	"google.golang.org/protobuf/proto"
)

// DnstapCallback is called for each parsed dnstap event.
type DnstapCallback func(*UnboundQueryLog)

// DnstapReader listens on a Unix socket for dnstap Frame Streams
// from Unbound and converts them to UnboundQueryLog entries.
type DnstapReader struct {
	socketPath string
	callback   DnstapCallback
	logger     *logging.Logger
	listener   net.Listener
	wg         sync.WaitGroup
	mu         sync.Mutex
	stopped    bool
}

// NewDnstapReader creates a new reader that will listen on the given
// Unix socket path. The callback is invoked for each parsed event.
func NewDnstapReader(socketPath string, callback DnstapCallback, logger *logging.Logger) *DnstapReader {
	return &DnstapReader{
		socketPath: socketPath,
		callback:   callback,
		logger:     logger,
	}
}

// Start creates the Unix socket and begins accepting connections.
// Must be called BEFORE Unbound starts (Unbound connects to this socket).
func (r *DnstapReader) Start(ctx context.Context) error {
	// Remove stale socket file
	_ = os.Remove(r.socketPath)

	listener, err := net.Listen("unix", r.socketPath)
	if err != nil {
		return fmt.Errorf("dnstap listen on %s: %w", r.socketPath, err)
	}

	// Make socket group-writable so Unbound (potentially different user) can connect.
	// Avoids world-writable (0666) which would let any local process inject fabricated messages.
	if err := os.Chmod(r.socketPath, 0660); err != nil {
		_ = listener.Close()
		return fmt.Errorf("chmod dnstap socket: %w", err)
	}

	r.mu.Lock()
	r.listener = listener
	r.mu.Unlock()

	r.logger.Info("dnstap reader started", "socket", r.socketPath)

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.acceptLoop(ctx)
	}()

	return nil
}

// Stop closes the listener and waits for all connections to drain.
func (r *DnstapReader) Stop() {
	r.mu.Lock()
	r.stopped = true
	if r.listener != nil {
		_ = r.listener.Close()
	}
	r.mu.Unlock()

	r.wg.Wait()
	_ = os.Remove(r.socketPath)
	r.logger.Info("dnstap reader stopped")
}

func (r *DnstapReader) acceptLoop(ctx context.Context) {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			r.mu.Lock()
			stopped := r.stopped
			r.mu.Unlock()
			if stopped {
				return
			}
			// Transient error — retry after brief pause
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		r.logger.Info("dnstap client connected")
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.handleConnection(ctx, conn)
		}()
	}
}

func (r *DnstapReader) handleConnection(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	decoder, err := framestream.NewDecoder(conn, &framestream.DecoderOptions{
		ContentType:   []byte("protobuf:dnstap.Dnstap"),
		Bidirectional: true,
	})
	if err != nil {
		r.logger.Error("dnstap decoder init failed", "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		frame, err := decoder.Decode()
		if err != nil {
			r.mu.Lock()
			stopped := r.stopped
			r.mu.Unlock()
			if !stopped {
				r.logger.Debug("dnstap connection closed", "error", err)
			}
			return
		}

		var tap dnstap.Dnstap
		if err := proto.Unmarshal(frame, &tap); err != nil {
			r.logger.Debug("dnstap unmarshal error", "error", err)
			continue
		}

		msg := tap.GetMessage()
		if msg == nil {
			continue
		}

		entry := r.convertMessage(msg)
		if entry != nil && r.callback != nil {
			r.callback(entry)
		}
	}
}

func (r *DnstapReader) convertMessage(msg *dnstap.Message) *UnboundQueryLog {
	entry := &UnboundQueryLog{
		MessageType: msg.GetType().String(),
	}

	msgType := msg.GetType()

	switch msgType {
	case dnstap.Message_CLIENT_QUERY, dnstap.Message_RESOLVER_QUERY:
		wireMsg := msg.GetQueryMessage()
		if wireMsg == nil {
			return nil
		}
		var dnsMsg dns.Msg
		if err := dnsMsg.Unpack(wireMsg); err != nil || len(dnsMsg.Question) == 0 {
			return nil
		}
		entry.Domain = dnsMsg.Question[0].Name
		entry.QueryType = dns.TypeToString[dnsMsg.Question[0].Qtype]
		if entry.QueryType == "" {
			entry.QueryType = fmt.Sprintf("TYPE%d", dnsMsg.Question[0].Qtype)
		}
		entry.Timestamp = nsecToTime(msg.GetQueryTimeSec(), msg.GetQueryTimeNsec())

	case dnstap.Message_CLIENT_RESPONSE, dnstap.Message_RESOLVER_RESPONSE:
		wireMsg := msg.GetResponseMessage()
		if wireMsg == nil {
			return nil
		}
		var dnsMsg dns.Msg
		if err := dnsMsg.Unpack(wireMsg); err != nil || len(dnsMsg.Question) == 0 {
			return nil
		}
		entry.Domain = dnsMsg.Question[0].Name
		entry.QueryType = dns.TypeToString[dnsMsg.Question[0].Qtype]
		if entry.QueryType == "" {
			entry.QueryType = fmt.Sprintf("TYPE%d", dnsMsg.Question[0].Qtype)
		}
		entry.ResponseCode = dns.RcodeToString[dnsMsg.Rcode]
		entry.DNSSECValidated = dnsMsg.AuthenticatedData
		entry.AnswerCount = len(dnsMsg.Answer)
		entry.ResponseSize = len(wireMsg)
		entry.Timestamp = nsecToTime(msg.GetResponseTimeSec(), msg.GetResponseTimeNsec())

		// Calculate duration from query time to response time
		if msg.QueryTimeSec != nil && msg.ResponseTimeSec != nil {
			queryTime := nsecToTime(msg.GetQueryTimeSec(), msg.GetQueryTimeNsec())
			duration := entry.Timestamp.Sub(queryTime)
			entry.DurationMs = float64(duration.Microseconds()) / 1000.0
		}

	default:
		return nil
	}

	// Extract addresses
	if addr := msg.GetQueryAddress(); len(addr) > 0 {
		entry.ClientIP = net.IP(addr).String()
	}
	if addr := msg.GetResponseAddress(); len(addr) > 0 {
		entry.ServerIP = net.IP(addr).String()
	}

	return entry
}

func nsecToTime(sec uint64, nsec uint32) time.Time {
	return time.Unix(int64(sec), int64(nsec))
}
