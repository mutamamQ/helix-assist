package util

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/leona/helix-assist/internal/config"
	"github.com/leona/helix-assist/internal/lsp"
)

type ProgressIndicator struct {
	svc            *lsp.Service
	enabled        bool
	updateInterval time.Duration
	spinnerFrames  []string
	ctx            context.Context
	cancel         context.CancelFunc
	startTime      time.Time
	done           chan struct{}
	shown          bool
	mu             sync.Mutex
}

func NewProgressIndicator(svc *lsp.Service, cfg *config.Config) *ProgressIndicator {
	return &ProgressIndicator{
		svc:            svc,
		enabled:        cfg.EnableProgressSpinner,
		updateInterval: time.Duration(cfg.ProgressUpdateInterval) * time.Millisecond,
		spinnerFrames:  []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	}
}

func (p *ProgressIndicator) Start() {
	if !p.enabled {
		return
	}

	p.mu.Lock()
	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.startTime = time.Now()
	p.done = make(chan struct{})
	p.mu.Unlock()

	go p.animate()
}

func (p *ProgressIndicator) Stop() {
	if !p.enabled {
		return
	}

	p.mu.Lock()
	cancel, done := p.cancel, p.done
	p.cancel = nil
	p.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done // no frame can be sent after this point
	if p.shown {
		// replace the last frame, otherwise it stays on the statusline looking hung
		p.svc.SendShowMessage(lsp.MessageTypeInfo, fmt.Sprintf(" done (%s)", p.formatElapsed(time.Since(p.startTime))))
	}
}

func (p *ProgressIndicator) animate() {
	ticker := time.NewTicker(p.updateInterval)
	defer ticker.Stop()
	defer close(p.done)

	frameIndex := 0

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			elapsed := time.Since(p.startTime)
			message := fmt.Sprintf(" %s (%s)", p.spinnerFrames[frameIndex], p.formatElapsed(elapsed))

			p.svc.SendShowMessage(lsp.MessageTypeInfo, message)
			p.shown = true

			frameIndex = (frameIndex + 1) % len(p.spinnerFrames)
		}
	}
}

func (p *ProgressIndicator) formatElapsed(duration time.Duration) string {
	seconds := duration.Seconds()
	return fmt.Sprintf("%.1fs", seconds)
}
