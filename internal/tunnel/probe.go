package tunnel

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

const (
	probeRequestSize     = 16
	probeResponseSize    = 21
	maxProbeDatagramSize = 64 * 1024
	defaultProbeLimit    = time.Second
)

var ErrProbeTimeout = errors.New("tunnel probe timeout")

type ProbeResult struct {
	ClientRTT    time.Duration
	ServerRTT    time.Duration
	LimitReached bool
}

type Prober struct {
	Timeout time.Duration
}

func (p Prober) Probe(ctx context.Context, endpoint Endpoint) (ProbeResult, error) {
	address, err := ProbeAddress(endpoint.Address)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("resolve probe address: %w", err)
	}

	timeout := p.Timeout
	if timeout <= 0 {
		timeout = defaultProbeLimit
	}

	token := make([]byte, probeRequestSize)
	if _, err := rand.Read(token); err != nil {
		return ProbeResult{}, fmt.Errorf("create probe uuid: %w", err)
	}
	token[6] = (token[6] & 0x0f) | 0x40
	token[8] = (token[8] & 0x3f) | 0x80

	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	dialer := net.Dialer{Deadline: deadline}
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return ProbeResult{}, classifyProbeError(ctx, "dial", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(deadline); err != nil {
		return ProbeResult{}, fmt.Errorf("set probe deadline: %w", err)
	}
	stopContextWatch := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
	})
	defer stopContextWatch()

	started := time.Now()
	if _, err := conn.Write(token); err != nil {
		return ProbeResult{}, classifyProbeError(ctx, "send", err)
	}

	response := make([]byte, maxProbeDatagramSize)
	for {
		n, err := conn.Read(response)
		if err != nil {
			return ProbeResult{}, classifyProbeError(ctx, "receive", err)
		}

		// A socket may receive a delayed response to another request. Keep the
		// original deadline and wait for the response carrying our UUID.
		if n != probeResponseSize ||
			!bytes.Equal(response[:probeRequestSize], token) {
			continue
		}

		serverMillis := int32(binary.BigEndian.Uint32(response[16:20]))
		if serverMillis < 0 {
			return ProbeResult{}, fmt.Errorf("negative server rtt %d", serverMillis)
		}
		return ProbeResult{
			ClientRTT:    time.Since(started),
			ServerRTT:    time.Duration(serverMillis) * time.Millisecond,
			LimitReached: response[20] != 0,
		}, nil
	}
}

func classifyProbeError(ctx context.Context, operation string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s probe packet: %w", operation, ctxErr)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%s: %w", operation, ErrProbeTimeout)
	}
	return fmt.Errorf("%s probe packet: %w", operation, err)
}
