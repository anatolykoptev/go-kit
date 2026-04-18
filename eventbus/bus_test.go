package eventbus

import (
	"context"
	"testing"
	"time"
)

func TestBus_PublishDelivers(t *testing.T) {
	b := New()
	ch := b.Subscribe("alerts.*")
	b.Publish("alerts.twitter", "account_locked")
	select {
	case e := <-ch:
		if e.Topic != "alerts.twitter" || e.Payload != "account_locked" {
			t.Fatalf("got %+v", e)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("event not received")
	}
}

func TestBus_SubscribeCtxCancelCloses(t *testing.T) {
	b := New()
	ctx, cancel := context.WithCancel(context.Background())
	ch := b.SubscribeCtx(ctx, "t")
	cancel()
	time.Sleep(10 * time.Millisecond)
	b.Publish("t", "x")
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after ctx cancel")
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected closed channel signal")
	}
}

func TestBus_DropOnFullBuffer(t *testing.T) {
	b := New()
	_ = b.Subscribe("t")
	// publish more than buffer size — must not block
	done := make(chan struct{})
	go func() {
		for i := 0; i < subscriberBufSize+100; i++ {
			b.Publish("t", i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Publish blocked on full buffer (should drop)")
	}
}

func TestBus_NoMatch(t *testing.T) {
	b := New()
	ch := b.Subscribe("a.*")
	b.Publish("b.x", "payload")
	select {
	case e := <-ch:
		t.Fatalf("unexpected delivery: %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}
