package tunnel

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestProbeDecodesResponseAndLimit(t *testing.T) {
	t.Parallel()

	endpoint := udpEndpoint(t, func(request []byte) []byte {
		if len(request) != probeRequestSize {
			t.Errorf("request size = %d, want %d", len(request), probeRequestSize)
		}
		if request[6]&0xf0 != 0x40 || request[8]&0xc0 != 0x80 {
			t.Errorf("request is not an RFC 4122 UUIDv4: %x", request)
		}
		response := make([]byte, probeResponseSize)
		copy(response, request)
		binary.BigEndian.PutUint32(response[16:20], uint32(37))
		response[20] = 1
		return response
	})

	result, err := (Prober{Timeout: time.Second}).Probe(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("Probe(): %v", err)
	}
	if result.ServerRTT != 37*time.Millisecond {
		t.Fatalf("server RTT = %s, want 37ms", result.ServerRTT)
	}
	if !result.LimitReached {
		t.Fatal("limit flag was not decoded")
	}
}

func TestProbeIgnoresMismatchedUUIDBeforeMatchingResponse(t *testing.T) {
	t.Parallel()

	endpoint := udpEndpointResponses(t, func(request []byte) [][]byte {
		mismatched := make([]byte, probeResponseSize)
		copy(mismatched, request)
		mismatched[0] ^= 0xff

		matching := make([]byte, probeResponseSize)
		copy(matching, request)
		binary.BigEndian.PutUint32(matching[16:20], uint32(23))
		return [][]byte{mismatched, matching}
	})

	result, err := (Prober{Timeout: time.Second}).Probe(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("Probe(): %v", err)
	}
	if result.ServerRTT != 23*time.Millisecond {
		t.Fatalf("server RTT = %s, want 23ms", result.ServerRTT)
	}
}

func TestProbeIgnoresMalformedResponseBeforeMatchingResponse(t *testing.T) {
	t.Parallel()

	endpoint := udpEndpointResponses(t, func(request []byte) [][]byte {
		matching := make([]byte, probeResponseSize)
		copy(matching, request)
		return [][]byte{{0x01, 0x02, 0x03}, matching}
	})

	if _, err := (Prober{Timeout: time.Second}).Probe(
		context.Background(),
		endpoint,
	); err != nil {
		t.Fatalf("Probe(): %v", err)
	}
}

func TestProbeIgnoresOversizedResponseBeforeMatchingResponse(t *testing.T) {
	t.Parallel()

	endpoint := udpEndpointResponses(t, func(request []byte) [][]byte {
		oversized := make([]byte, 1024)
		copy(oversized, request)
		matching := make([]byte, probeResponseSize)
		copy(matching, request)
		return [][]byte{oversized, matching}
	})

	if _, err := (Prober{Timeout: time.Second}).Probe(
		context.Background(),
		endpoint,
	); err != nil {
		t.Fatalf("Probe(): %v", err)
	}
}

func TestProbeTimesOutAfterOnlyMismatchedUUID(t *testing.T) {
	t.Parallel()

	endpoint := udpEndpoint(t, func(request []byte) []byte {
		response := make([]byte, probeResponseSize)
		copy(response, request)
		response[0] ^= 0xff
		return response
	})

	_, err := (Prober{Timeout: 50 * time.Millisecond}).Probe(
		context.Background(),
		endpoint,
	)
	if !errors.Is(err, ErrProbeTimeout) {
		t.Fatalf("Probe() error = %v, want ErrProbeTimeout", err)
	}
}

func TestProbeTimesOut(t *testing.T) {
	t.Parallel()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("ListenUDP(): %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	endpoint := endpointForProbePort(t, conn.LocalAddr().(*net.UDPAddr).Port)
	_, err = (Prober{Timeout: 25 * time.Millisecond}).Probe(context.Background(), endpoint)
	if !errors.Is(err, ErrProbeTimeout) {
		t.Fatalf("Probe() error = %v, want ErrProbeTimeout", err)
	}
}

func TestProbeStopsWhenContextIsCanceled(t *testing.T) {
	t.Parallel()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("ListenUDP(): %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, cancel)

	started := time.Now()
	endpoint := endpointForProbePort(t, conn.LocalAddr().(*net.UDPAddr).Port)
	_, err = (Prober{Timeout: time.Second}).Probe(ctx, endpoint)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Probe() error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("Probe() stopped after %s, want prompt cancellation", elapsed)
	}
}

func TestProbeRepeatedRoundsRejectPriorRepliesAndPreserveServerRTT(t *testing.T) {
	t.Parallel()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("ListenUDP(): %v", err)
	}
	done := make(chan struct{})
	serverMillis := []uint32{15, 15, 15, 1_500_000, 15, 15}
	go func() {
		defer close(done)
		var previous []byte
		request := make([]byte, probeRequestSize+1)
		for _, millis := range serverMillis {
			n, peer, readErr := conn.ReadFromUDP(request)
			if readErr != nil {
				return
			}
			if n != probeRequestSize {
				t.Errorf("request size = %d, want %d", n, probeRequestSize)
				return
			}
			if previous != nil {
				if _, writeErr := conn.WriteToUDP(previous, peer); writeErr != nil {
					return
				}
			}
			response := make([]byte, probeResponseSize)
			copy(response, request[:n])
			binary.BigEndian.PutUint32(response[16:20], millis)
			if _, writeErr := conn.WriteToUDP(response, peer); writeErr != nil {
				return
			}
			previous = response
		}
	}()
	t.Cleanup(func() {
		_ = conn.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("UDP responder did not stop")
		}
	})

	target := testTarget(endpointForProbePort(t, conn.LocalAddr().(*net.UDPAddr).Port).Address)
	cache := NewCache()
	for round, millis := range serverMillis {
		result, probeErr := (Prober{Timeout: time.Second}).Probe(context.Background(), target.Endpoint)
		if probeErr != nil {
			t.Fatalf("round %d: %v", round+1, probeErr)
		}
		if result.ServerRTT != time.Duration(millis)*time.Millisecond {
			t.Fatalf("round %d server RTT = %s, want %dms from current reply", round+1, result.ServerRTT, millis)
		}
		cache.Record(target, result, nil, time.Now())
	}
	history, ok := cache.History(target.Region, target.Endpoint.Address)
	if !ok || len(history.Samples) != len(serverMillis) || history.Samples[3].ServerRTTMillis != 1_500_000 ||
		history.Samples[5].ServerRTTMillis != 15 {
		t.Fatalf("raw server-reported history = %#v", history)
	}
	metrics, ok := cache.Metrics(target.Region, target.Endpoint.Address, time.Now())
	if !ok || metrics.ServerRTT != 250_012.5 {
		t.Fatalf("history server RTT = %v, want 250012.5ms, distinct from latest 15ms", metrics.ServerRTT)
	}
}

func udpEndpoint(t *testing.T, respond func([]byte) []byte) Endpoint {
	t.Helper()

	return udpEndpointResponses(t, func(request []byte) [][]byte {
		return [][]byte{respond(request)}
	})
}

func udpEndpointResponses(
	t *testing.T,
	respond func([]byte) [][]byte,
) Endpoint {
	t.Helper()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("ListenUDP(): %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		request := make([]byte, probeRequestSize)
		n, peer, readErr := conn.ReadFromUDP(request)
		if readErr != nil {
			return
		}
		for _, response := range respond(request[:n]) {
			if _, writeErr := conn.WriteToUDP(response, peer); writeErr != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = conn.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("UDP responder did not stop")
		}
	})

	return endpointForProbePort(t, conn.LocalAddr().(*net.UDPAddr).Port)
}

func endpointForProbePort(t *testing.T, probePort int) Endpoint {
	t.Helper()
	if probePort <= 1 {
		t.Fatalf("unexpected probe port %d", probePort)
	}
	return Endpoint{
		Name:    "test",
		Address: net.JoinHostPort("127.0.0.1", strconv.Itoa(probePort-1)),
	}
}
