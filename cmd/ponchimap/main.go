// Command ponchimap leaves a diagram of the infrastructure an app uses
// and a report in the repository it runs in.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ochamu/ponchimap/internal/cli"
)

func main() {
	// Ctrl-C and SIGTERM cancel the run instead of killing it, so that a
	// render stops before it saves or saves every file.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// A layout engine may not look at the context for a long time. After the
	// first signal the signals act as usual again, so a second Ctrl-C ends
	// the run at once.
	go func() {
		<-ctx.Done()
		stop()
	}()
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
