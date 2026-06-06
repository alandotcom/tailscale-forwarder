package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"syscall"
	"time"
)

// fwdTCP dials the target and copies bytes in both directions between the
// source and target connections. It returns as soon as either copy direction
// finishes (a normal peer close, an error) or ctx is cancelled, then closes
// both connections so the surviving copy goroutine unblocks and exits. This
// keeps the number of live goroutines bounded by the number of *concurrent*
// connections rather than the total ever served.
func fwdTCP(ctx context.Context, sourceConn net.Conn, targetAddr string, targetPort int) error {
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	targetConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(targetAddr, strconv.Itoa(targetPort)))
	if err != nil {
		return fmt.Errorf("failed to dial target (%s): %w", dialErrorReason(err), err)
	}

	// Buffered so both goroutines can always send their result and exit, even
	// if we have already stopped reading from the channel.
	done := make(chan error, 2)
	go func() { _, err := io.Copy(targetConn, sourceConn); done <- err }()
	go func() { _, err := io.Copy(sourceConn, targetConn); done <- err }()

	var copyErr error
	select {
	case <-ctx.Done():
	case copyErr = <-done:
	}

	// Closing both ends unblocks the still-running copy direction.
	sourceConn.Close()
	targetConn.Close()

	if copyErr != nil && !errors.Is(copyErr, io.EOF) && !errors.Is(copyErr, net.ErrClosed) {
		return copyErr
	}
	return nil
}

// dialErrorReason classifies a dial failure so operators can tell "target down"
// (refused) from "target slow/unreachable" (timeout) from "name resolution
// broken" (dns) without reproducing it by hand.
func dialErrorReason(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns_error"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "connection_refused"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	return "dial_error"
}
