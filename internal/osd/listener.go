package osd

import (
	"context"
	"sync"
	"time"
)

type ListenerController struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func StartListeners(o *OSD) *ListenerController {
	if o == nil {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	lc := &ListenerController{
		cancel: cancel,
	}

	lc.wg.Add(2)
	go func() {
		defer lc.wg.Done()
		runAudioListener(ctx, o)
	}()

	go func() {
		defer lc.wg.Done()
		runBacklightListener(ctx, o)
	}()

	return lc
}

func (lc *ListenerController) Stop() {
	if lc == nil {
		return
	}
	lc.cancel()

	done := make(chan struct{})
	go func() {
		lc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
	}
}
