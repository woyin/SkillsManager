package cmd

import "testing"

func TestWebListenAddrBindsLoopback(t *testing.T) {
	if got := webListenAddr(3721); got != "127.0.0.1:3721" {
		t.Fatalf("webListenAddr(3721) = %q, want 127.0.0.1:3721", got)
	}
	if got := webListenAddr(8080); got != "127.0.0.1:8080" {
		t.Fatalf("webListenAddr(8080) = %q, want 127.0.0.1:8080", got)
	}
}
