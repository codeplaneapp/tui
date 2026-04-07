package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/goleak"
)

type blockingTransport struct {
	mcp.Transport
	closeBlock chan struct{}
}

func (t *blockingTransport) Close() error {
	<-t.closeBlock
	return nil
}

func TestClose_GoroutineLeak(t *testing.T) {
	// We want to see if goleak detects a leak.
	// Since we expect a leak, we might want to check it manually first or
	// use a subtest.
	
	// goleak.VerifyNone(t) // This will fail if there is a leak.

	// Setup a session with a blocking transport.
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	
	// We need a transport that blocks on Close.
	// mcp.InMemoryTransport doesn't easily allow blocking on Close.
	// But we can wrap it.
	c1, _ := mcp.NewInMemoryTransports()
	
	closeBlock := make(chan struct{})
	bt := &blockingTransport{
		Transport:  c1,
		closeBlock: closeBlock,
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	session, err := client.Connect(ctx, bt, nil)
	if err != nil {
		t.Fatal(err)
	}
	
	sess := &ClientSession{
		ClientSession: session,
		cancel:        cancel,
	}
	
	sessions.Set("leaky", sess)
	
	// Call Close with a short timeout.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer shutdownCancel()
	
	Close(shutdownCtx)
	
	// Now, if we don't close closeBlock, the goroutine is leaked.
	// goleak should find it.
	
	err = goleak.Find(goleak.IgnoreCurrent())
	if err == nil {
		t.Error("Expected to find a goroutine leak, but none was found")
	} else {
		t.Logf("Found expected leak: %v", err)
	}
	
	// Cleanup to avoid affecting other tests.
	close(closeBlock)
	sessions.Del("leaky")
	
	// Wait a bit for it to actually finish.
	time.Sleep(50*time.Millisecond)
	
	if err := goleak.Find(goleak.IgnoreCurrent()); err != nil {
		t.Errorf("Leak still present after cleanup: %v", err)
	}
}
