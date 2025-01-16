# syswrap

`syswrap` is a small program to wrap system calls with timeouts. In Go, syscalls cannot be cleaned up if they are
blocked, for example if `lstat` is called on an inaccessible NFS mount, but because Go can create additional goroutines
the calling application will not block. This can cause resource exhaustion if syscalls are not bounded.

Linux will clean up syscalls if the owning process ends, so `syswrap` exists to allow Trident to create a new process
that owns a potentially blocking syscall.

The Trident-accessible interface is in this package.

## Usage

`syswrap <timeout> <syscall> <syscall args...>`

`<timeout>` is in Go format, i.e. `30s`

`<syscall>` is the name of the call, see `cmd/syswrap/main.go`

`<syscall args...>` are string representations of any arguments required by the call.

## Adding Syscalls

See `syswrap.Exists` for a cross-platform example, and `syswrap.Statfs` for a Linux-only example.

There are 3 steps to adding a new syscall:

1. Add func to `internal/syswrap` package. This func calls the syswrap binary, and it should also call the syscall
itself if the syswrap binary is not found.
2. Add func to `internal/syswrap/unix` package. The func must have the signature
`func(args []string) (output interface{}, err error)`, and parse its args then call the syscall. Any fields in output
that need to be used by Trident must be exported.
3. Add func name to `cmd/syswrap/main.syscalls` map.