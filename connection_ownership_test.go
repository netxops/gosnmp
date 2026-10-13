package gosnmp

import (
	"errors"
	"net"
	"testing"
	"time"
)

type ownershipTestConn struct {
	closeErr error
	closes   int
}

func (c *ownershipTestConn) Read([]byte) (int, error)       { return 0, errors.New("unused") }
func (c *ownershipTestConn) Write([]byte) (int, error)      { return 0, errors.New("unused") }
func (c *ownershipTestConn) Close() error                   { c.closes++; return c.closeErr }
func (*ownershipTestConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (*ownershipTestConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (*ownershipTestConn) SetDeadline(time.Time) error      { return nil }
func (*ownershipTestConn) SetReadDeadline(time.Time) error  { return nil }
func (*ownershipTestConn) SetWriteDeadline(time.Time) error { return nil }

func TestCloseFailureRetainsOwnedConnectionForRetry(t *testing.T) {
	closeErr := errors.New("socket exit unproven")
	conn := &ownershipTestConn{closeErr: closeErr}
	client := &GoSNMP{Conn: conn}
	if err := client.Close(); !errors.Is(err, closeErr) || client.Conn != conn {
		t.Fatal("close failure must retain actual owned connection")
	}
	conn.closeErr = nil
	if err := client.Close(); err != nil || client.Conn != nil || conn.closes != 2 {
		t.Fatal("second close must actually retry before releasing ownership")
	}
}

func TestTCPReplacementRefusesUnprovenOldExit(t *testing.T) {
	closeErr := errors.New("socket exit unproven")
	conn := &ownershipTestConn{closeErr: closeErr}
	client := &GoSNMP{Conn: conn, Transport: "tcp", Target: "192.0.2.1", Port: 161}
	// Refusal occurs before any dial, DNS, or replacement. EOF at the caller
	// cannot bypass this same ownership gate.
	if err := client.netConnect(); !errors.Is(err, closeErr) || client.Conn != conn || conn.closes != 1 {
		t.Fatal("reconnect must retain failed old connection and refuse new ownership")
	}
}

func TestAlreadyClosedTransportReleasesOwnership(t *testing.T) {
	conn := &ownershipTestConn{closeErr: net.ErrClosed}
	client := &GoSNMP{Conn: conn}
	if err := client.Close(); err != nil || client.Conn != nil {
		t.Fatal("explicit transport already-closed proof should release ownership")
	}
}

func TestRetainedCloseFailureDoesNotCorruptExplicitFamilyRetry(t *testing.T) {
	closeErr := errors.New("socket exit unproven")
	conn := &ownershipTestConn{closeErr: closeErr}
	client := &GoSNMP{Conn: conn, Transport: "tcp4", Target: "192.0.2.1", Port: 161}
	for attempt := 0; attempt < 2; attempt++ {
		if err := client.ConnectIPv4(); !errors.Is(err, closeErr) || client.Conn != conn || client.Transport != "tcp4" {
			t.Fatal("explicit IPv4 retry must preserve owned connection and correct transport")
		}
	}
}
