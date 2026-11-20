package service

import (
	"context"
	"testing"
	"time"
)

func TestWorkerFollowsAppliedMenuChanges(t *testing.T) {
	s := testService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan string, 4)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		s.RunMenuWorker(ctx, "errors", func(ctx context.Context) {
			events <- "start"
			<-ctx.Done()
			events <- "stop"
		})
	}()
	expect := func(want string) {
		t.Helper()
		select {
		case got := <-events:
			if got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("worker did not %s", want)
		}
	}
	expect("start")
	settings := s.State().Settings
	settings.HiddenMenus = []string{"errors"}
	if err := s.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	if !s.MenuEnabled("errors") {
		t.Fatal("draft stopped the active worker")
	}
	s.setAppliedMenus(settings.HiddenMenus)
	expect("stop")
	s.setAppliedMenus(nil)
	expect("start")
	cancel()
	expect("stop")
	<-stopped
}
