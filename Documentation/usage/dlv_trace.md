## dlv trace

Compile and begin tracing program.

### Synopsis


Trace program execution.

The trace sub command will set a tracepoint on every function matching the
provided regular expression and output information when tracepoint is hit.  This
is useful if you do not want to begin an entire debug session, but merely want
to know what functions your process is executing.

```
dlv trace [package] regexp
```

### Options

```
  -e, --exec string     Binary file to exec and trace.
      --output string   Output path for the binary. (default "debug")
  -p, --pid int         Pid to attach to.
  -s, --stack int       Show stack trace with given depth.
  -t, --test            Trace a test binary.
```

### Options inherited from parent commands

```
      --accept-multiclient   Allows a headless server to accept multiple client connections.
      --api-version int      Selects API version when headless. (default 1)
      --backend string       Backend selection (see 'dlv help backend'). (default "default")
      --build-flags string   Build flags, to be passed to the compiler.
      --check-go-version     Checks that the version of Go in use is compatible with Delve. (default true)
      --headless             Run debug server only, in headless mode.
      --init string          Init file, executed by the terminal client.
  -l, --listen string        Debugging server listen address. (default "127.0.0.1:0")
      --log                  Enable debugging server logging.
      --log-dest string      Writes logs to the specified file or file descriptor (see 'dlv help log').
      --log-output string    Comma separated list of components that should produce debug output (see 'dlv help log')
      --wd string            Working directory for running the program. (default ".")
```

### SEE ALSO
* [dlv](dlv.md)	 - Delve is a debugger for the Go programming language.

