package cli

import (
	"errors"
	"flag"
	"math"
	"strconv"

	"github.com/ochamu/ponchimap/internal/view"
)

// addLayoutFlags adds the flags that replace a value of the layout in the
// view config for one render. A flag left out leaves its field zero, which
// keeps the value of the view config. Each flag refuses a value that names no
// layout itself, so that the message names the flag and not the view config.
func addLayoutFlags(flags *flag.FlagSet, override *view.LayoutConfig) {
	flags.Var((*engineFlag)(&override.Engine), "engine", "draw with this layout `engine`, tala or elk (default: the one in the view config)")
	flags.Var((*directionFlag)(&override.Direction), "direction", "draw in this `direction`, right or down (default: the one in the view config)")
	flags.Var((*seedFlag)(&override.Seed), "seed", "draw with this TALA seed, a whole `number` from 1 to 2147483647 (default: the one in the view config)")
}

type engineFlag view.Engine

func (e *engineFlag) String() string { return string(*e) }

func (e *engineFlag) Set(s string) error {
	switch view.Engine(s) {
	case view.EngineTALA, view.EngineELK:
		*e = engineFlag(s)
		return nil
	}
	return errors.New("an engine is tala or elk")
}

type directionFlag view.Direction

func (d *directionFlag) String() string { return string(*d) }

func (d *directionFlag) Set(s string) error {
	switch view.Direction(s) {
	case view.DirRight, view.DirDown:
		*d = directionFlag(s)
		return nil
	}
	return errors.New("a direction is right or down")
}

type seedFlag view.Seed

func (s *seedFlag) String() string { return strconv.FormatInt(int64(*s), 10) }

// The range is the one the schema of the view config gives layout.seed.
func (s *seedFlag) Set(text string) error {
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || n < 1 || n > math.MaxInt32 {
		return errors.New("a seed is a whole number from 1 to 2147483647")
	}
	*s = seedFlag(n)
	return nil
}
