package kernel

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

type Lifecycle struct {
	ctx       context.Context
	cancel    context.CancelFunc
	shutdown  chan struct{}
}

func NewLifecycle() *Lifecycle {
	ctx, cancel := context.WithCancel(context.Background())
	return &Lifecycle{
		ctx:      ctx,
		cancel:   cancel,
		shutdown: make(chan struct{}),
	}
}

func (l *Lifecycle) Context() context.Context {
	return l.ctx
}

func (l *Lifecycle) ShutdownChan() <-chan struct{} {
	return l.shutdown
}

func (l *Lifecycle) WaitForSignal(signals ...os.Signal) {
	sigCh := make(chan os.Signal, 1)
	if len(signals) == 0 {
		signals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	}
	signal.Notify(sigCh, signals...)
	go func() {
		<-sigCh
		l.GracefulShutdown()
	}()
}

func (l *Lifecycle) GracefulShutdown() {
	l.cancel()
	close(l.shutdown)
}

func (l *Lifecycle) IsShuttingDown() bool {
	select {
	case <-l.ctx.Done():
		return true
	default:
		return false
	}
}
