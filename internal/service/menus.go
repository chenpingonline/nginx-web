package service

import (
	"context"
	"slices"
)

func (s *AppService) setAppliedMenus(hidden []string) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if s.menuChanged != nil {
		if slices.Equal(s.appliedMenus, hidden) {
			return
		}
		close(s.menuChanged)
	}
	s.appliedMenus = slices.Clone(hidden)
	s.menuChanged = make(chan struct{})
}

func (s *AppService) hiddenMenus() []string {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	return slices.Clone(s.appliedMenus)
}

func (s *AppService) MenuEnabled(menu string) bool {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	return !slices.Contains(s.appliedMenus, menu)
}

// Wait for configuration changes while disabled. Stop the old worker before
// re-enabling it so collectors never compete for the same files or socket.
func (s *AppService) RunMenuWorker(ctx context.Context, menu string, run func(context.Context)) {
	var stop context.CancelFunc
	var done chan struct{}
	shutdown := func() {
		if stop != nil {
			stop()
			<-done
			stop = nil
		}
	}
	defer shutdown()
	for ctx.Err() == nil {
		s.authMu.RLock()
		enabled, changed := !slices.Contains(s.appliedMenus, menu), s.menuChanged
		s.authMu.RUnlock()
		if enabled && stop == nil {
			var worker context.Context
			worker, stop = context.WithCancel(ctx)
			done = make(chan struct{})
			go func() { defer close(done); run(worker) }()
		} else if !enabled {
			shutdown()
		}
		select {
		case <-ctx.Done():
			return
		case <-changed:
		}
	}
}
