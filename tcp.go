package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
)

func fwdTCP(ctx context.Context, sourceConn net.Conn, targetAddr string, targetPort int) error {
	// Use context for dial timeout
	dialer := &net.Dialer{
		Timeout: 30 * time.Second,
	}
	targetConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(targetAddr, fmt.Sprintf("%d", targetPort)))
	if err != nil {
		return fmt.Errorf("failed to dial target: %w", err)
	}
	defer targetConn.Close()

	// Simple bidirectional copy - connections auto-close on context cancellation
	go func() {
		defer sourceConn.Close()
		defer targetConn.Close()
		io.Copy(targetConn, sourceConn)
	}()

	go func() {
		defer sourceConn.Close()
		defer targetConn.Close()
		io.Copy(sourceConn, targetConn)
	}()

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}
