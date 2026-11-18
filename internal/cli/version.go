package cli

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"

	"github.com/ochamu/ponchimap/internal/render/d2render"
)

func runVersion(_ context.Context, args []string, stdout, _ io.Writer) outcome {
	if o, stop := parseFlags(newFlagSet("version"), args, stdout); stop {
		return o
	}
	fmt.Fprintf(stdout, "%s %s, d2 %s, %s\n", tool, toolVersion(), d2render.Version(), runtime.Version())
	return outcome{}
}

// toolVersion is the version of the main module that Go put into the binary.
// A binary of go run or go test has none and is a development build.
func toolVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
