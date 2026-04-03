package kernel

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
)

// Shutdown performs a graceful shutdown of the kernel in 4 phases:
//  1. Stop accepting new HTTP connections
//  2. Drain in-flight HTTP requests
//  3. Shutdown modules in reverse dependency order
//  4. Close infrastructure connections (Postgres, Redis)
//
// All errors are collected and returned as a joined error.
func (k *Kernel) Shutdown(ctx context.Context) error {
	var errs []error
	collect := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	k.shutdownOnce.Do(func() {
		k.logger.Info("shutting down kernel")

		// Stop and drain HTTP server.
		if k.httpServer != nil {
			k.httpServer.SetKeepAlivesEnabled(false)
			k.logger.Info("draining HTTP connections")
			collect(k.httpServer.Shutdown(ctx))
		}

		// Shutdown modules in reverse dependency order.
		for _, id := range k.reverseDepOrder() {
			for _, m := range k.modules {
				if m.Manifest().ID == id {
					k.logger.Info("shutting down module", "id", id)
					collect(m.Shutdown())
					break
				}
			}
		}

		// Close infrastructure connections.
		k.logger.Info("closing infrastructure connections")
		k.closeInfra()

		k.logger.Info("kernel shutdown complete")
	})

	return errors.Join(errs...)
}

// WaitForSignal blocks until SIGTERM or SIGINT is received,
// then triggers graceful shutdown with the configured timeout.
func (k *Kernel) WaitForSignal() error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	sig := <-quit
	signal.Stop(quit) // clean up signal handler

	k.logger.Info("received signal", "signal", sig.String())

	timeout := k.cfg.Server.ShutdownTimeout
	if timeout == 0 {
		timeout = DefaultConfig().Server.ShutdownTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return k.Shutdown(ctx)
}
