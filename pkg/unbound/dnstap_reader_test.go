package unbound

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	dnstap "github.com/dnstap/golang-dnstap"
	framestream "github.com/farsightsec/golang-framestream"
	"github.com/miekg/dns"
	"glory-hole/pkg/logging"
	"google.golang.org/protobuf/proto"
)

func TestDnstapReaderStartStop(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "dnstap.sock")
	logger := logging.NewDefault()

	reader := NewDnstapReader(sockPath, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := reader.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Verify socket exists
	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("socket not created: %v", err)
	}

	reader.Stop()

	// Verify socket cleaned up
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Fatal("socket not cleaned up after Stop")
	}
}

func TestDnstapReaderParsesClientResponse(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "dnstap.sock")
	logger := logging.NewDefault()

	var received []*UnboundQueryLog
	var mu sync.Mutex

	reader := NewDnstapReader(sockPath, func(entry *UnboundQueryLog) {
		mu.Lock()
		received = append(received, entry)
		mu.Unlock()
	}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := reader.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer reader.Stop()

	// Connect as a dnstap client (Unbound's role)
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	encoder, err := framestream.NewEncoder(conn, &framestream.EncoderOptions{
		ContentType:   []byte("protobuf:dnstap.Dnstap"),
		Bidirectional: true,
	})
	if err != nil {
		t.Fatalf("encoder: %v", err)
	}

	// Build a DNS response message
	dnsResp := new(dns.Msg)
	dnsResp.SetQuestion("example.com.", dns.TypeA)
	dnsResp.Rcode = dns.RcodeSuccess
	dnsResp.AuthenticatedData = true
	dnsResp.Answer = append(dnsResp.Answer, &dns.A{
		Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.ParseIP("93.184.216.34"),
	})

	wireResp, err := dnsResp.Pack()
	if err != nil {
		t.Fatalf("pack DNS: %v", err)
	}

	// Build dnstap CLIENT_RESPONSE message
	msgType := dnstap.Message_CLIENT_RESPONSE
	socketFamily := dnstap.SocketFamily_INET
	socketProto := dnstap.SocketProtocol_UDP
	queryAddr := net.ParseIP("127.0.0.1").To4()
	queryPort := uint32(12345)
	now := time.Now()
	querySec := uint64(now.Add(-5 * time.Millisecond).Unix())
	queryNsec := uint32(now.Add(-5 * time.Millisecond).Nanosecond())
	respSec := uint64(now.Unix())
	respNsec := uint32(now.Nanosecond())

	tapMsg := &dnstap.Dnstap{
		Type: dnstap.Dnstap_MESSAGE.Enum(),
		Message: &dnstap.Message{
			Type:             &msgType,
			SocketFamily:     &socketFamily,
			SocketProtocol:   &socketProto,
			QueryAddress:     queryAddr,
			QueryPort:        &queryPort,
			QueryTimeSec:     &querySec,
			QueryTimeNsec:    &queryNsec,
			ResponseTimeSec:  &respSec,
			ResponseTimeNsec: &respNsec,
			ResponseMessage:  wireResp,
		},
	}

	data, err := proto.Marshal(tapMsg)
	if err != nil {
		t.Fatalf("marshal dnstap: %v", err)
	}

	if _, err := encoder.Write(data); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	encoder.Flush()

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(received))
	}

	entry := received[0]

	if entry.MessageType != "CLIENT_RESPONSE" {
		t.Errorf("message_type = %q, want CLIENT_RESPONSE", entry.MessageType)
	}
	if entry.Domain != "example.com." {
		t.Errorf("domain = %q, want example.com.", entry.Domain)
	}
	if entry.QueryType != "A" {
		t.Errorf("query_type = %q, want A", entry.QueryType)
	}
	if entry.ResponseCode != "NOERROR" {
		t.Errorf("response_code = %q, want NOERROR", entry.ResponseCode)
	}
	if !entry.DNSSECValidated {
		t.Error("dnssec_validated = false, want true")
	}
	if entry.AnswerCount != 1 {
		t.Errorf("answer_count = %d, want 1", entry.AnswerCount)
	}
	if entry.ClientIP != "127.0.0.1" {
		t.Errorf("client_ip = %q, want 127.0.0.1", entry.ClientIP)
	}
	if entry.DurationMs <= 0 {
		t.Errorf("duration_ms = %f, want > 0", entry.DurationMs)
	}
}

func TestDnstapReaderParsesResolverQuery(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "dnstap.sock")
	logger := logging.NewDefault()

	var received []*UnboundQueryLog
	var mu sync.Mutex

	reader := NewDnstapReader(sockPath, func(entry *UnboundQueryLog) {
		mu.Lock()
		received = append(received, entry)
		mu.Unlock()
	}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := reader.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer reader.Stop()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	encoder, err := framestream.NewEncoder(conn, &framestream.EncoderOptions{
		ContentType:   []byte("protobuf:dnstap.Dnstap"),
		Bidirectional: true,
	})
	if err != nil {
		t.Fatalf("encoder: %v", err)
	}

	// Build a DNS query message (Unbound → authoritative NS)
	dnsQuery := new(dns.Msg)
	dnsQuery.SetQuestion("example.com.", dns.TypeA)

	wireQuery, err := dnsQuery.Pack()
	if err != nil {
		t.Fatalf("pack DNS: %v", err)
	}

	msgType := dnstap.Message_RESOLVER_QUERY
	socketFamily := dnstap.SocketFamily_INET
	socketProto := dnstap.SocketProtocol_UDP
	queryAddr := net.ParseIP("127.0.0.1").To4()
	respAddr := net.ParseIP("198.41.0.4").To4() // a.root-servers.net
	queryPort := uint32(53)
	now := time.Now()
	querySec := uint64(now.Unix())
	queryNsec := uint32(now.Nanosecond())

	tapMsg := &dnstap.Dnstap{
		Type: dnstap.Dnstap_MESSAGE.Enum(),
		Message: &dnstap.Message{
			Type:            &msgType,
			SocketFamily:    &socketFamily,
			SocketProtocol:  &socketProto,
			QueryAddress:    queryAddr,
			QueryPort:       &queryPort,
			ResponseAddress: respAddr,
			QueryTimeSec:    &querySec,
			QueryTimeNsec:   &queryNsec,
			QueryMessage:    wireQuery,
		},
	}

	data, err := proto.Marshal(tapMsg)
	if err != nil {
		t.Fatalf("marshal dnstap: %v", err)
	}

	if _, err := encoder.Write(data); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	encoder.Flush()

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(received))
	}

	entry := received[0]
	if entry.MessageType != "RESOLVER_QUERY" {
		t.Errorf("message_type = %q, want RESOLVER_QUERY", entry.MessageType)
	}
	if entry.Domain != "example.com." {
		t.Errorf("domain = %q, want example.com.", entry.Domain)
	}
	if entry.ServerIP != "198.41.0.4" {
		t.Errorf("server_ip = %q, want 198.41.0.4", entry.ServerIP)
	}
}
