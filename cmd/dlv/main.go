package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/undoio/delve/cmd/dlv/cmds"
	"github.com/undoio/delve/pkg/version"
	"github.com/sirupsen/logrus"
)

// Build is the git sha of this binaries build.
var Build string

func main() {
	if Build != "" {
		version.DelveVersion.Build = Build
	}
	const cgoCflagsEnv = "CGO_CFLAGS"
	if os.Getenv(cgoCflagsEnv) == "" {
		os.Setenv(cgoCflagsEnv, "-O0 -g")
	} else {
		logrus.WithFields(logrus.Fields{"layer": "dlv"}).Warnln("CGO_CFLAGS already set, Cgo code could be optimized.")
	}

	// XXX: There is currently no way to enforce the record/replay backend
	// from within GoLand.  Until this is resolved, introduce an environment
	// variable to override the backend.
	newArgs := make([]string, len(os.Args))
	dlvBackend := os.Getenv("DLV_RECORD_REPLAY_BACKEND")
	if dlvBackend != "" {
		if dlvBackend != "rr" && dlvBackend != "undo" {
			fmt.Fprintf(os.Stderr, "Unknown dlv record/replay backend: %s\n",
				dlvBackend)
			os.Exit(1)
		}

		replaceOptArg := false
		for i, arg := range os.Args {
			if replaceOptArg {
				if strings.Compare(arg, "rr") == 0 ||
					strings.Compare(arg, "undo") == 0 {
					newArgs[i] = dlvBackend
				} else {
					newArgs[i] = os.Args[i]
				}
				replaceOptArg = false
				continue
			}

			/* Handle --option XXX case */
			if strings.Compare(arg, "--backend") == 0 {
				newArgs[i] = arg
				replaceOptArg = true
				continue
			}

			/* Handle --option=XXX case */
			if strings.Compare(arg, "--backend=rr") == 0 ||
				strings.Compare(arg, "--backend=undo") == 0 {
				newArgs[i] = "--backend=" + dlvBackend
				continue
			}

			newArgs[i] = os.Args[i]
		}
		os.Args = newArgs
	}

	cmds.New(false).Execute()
}
