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

// fwdTCP preserves half-closes until both copy directions finish. Cancellation
// or a copy error closes both connections and waits for the copies to stop.
func fwdTCP(ctx context.Context, sourceConn net.Conn, targetAddr string, targetPort int) error {
	defer sourceConn.Close()
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	targetConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(targetAddr, strconv.Itoa(targetPort)))
	if err != nil {
		return fmt.Errorf("failed to dial target (%s): %w", dialErrorReason(err), err)
	}
	defer targetConn.Close()

	done := make(chan error, 2)
	copyConn := func(dst, src net.Conn) {
		_, err := io.Copy(dst, src)
		if err == nil {
			if writer, ok := dst.(interface{ CloseWrite() error }); ok {
				err = writer.CloseWrite()
			} else {
				err = dst.Close()
			}
		}
		done <- err
	}
	go copyConn(targetConn, sourceConn)
	go copyConn(sourceConn, targetConn)

	var copyErr error
	remaining := 2
copying:
	for remaining > 0 {
		select {
		case <-ctx.Done():
			break copying
		case err := <-done:
			remaining--
			if err != nil {
				copyErr = err
				break copying
			}
		}
	}

	sourceConn.Close()
	targetConn.Close()
	for remaining > 0 {
		<-done
		remaining--
	}

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
