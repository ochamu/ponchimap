package cli

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/ochamu/ponchimap/internal/render/d2render"
)

// Release builds set version with -ldflags -X when building from a checkout.
var version string

func runVersion(_ context.Context, args []string, stdout, _ io.Writer) outcome {
	if o, stop := parseFlags(newFlagSet("version"), args, stdout); stop {
		return o
	}
	fmt.Fprintf(stdout, "%s %s, d2 %s, %s\n", tool, toolVersion(), d2render.Version(), runtime.Version())
	return outcome{}
}

func toolVersion() string {
	if tag := strings.TrimSpace(version); tag != "" {
		return tag
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
