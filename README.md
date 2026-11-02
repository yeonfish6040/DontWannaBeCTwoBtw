# DontWannaBeCTwoBtw

Experimental Linux FUSE deception filesystem. It exposes a single generated text file containing every IPv4 address in numerical order, one address and LF per line, without storing the 61,337,501,696-byte stream on disk. **This is an early prototype, not production-ready security monitoring.**

## Build and run

Go 1.23+, Linux, FUSE and `fusermount3` are required. Build with `go build -o dwbc ./cmd/dwbc`. Create a dedicated empty mount directory and start:

```sh
mkdir -p /tmp/dwbc-mount
./dwbc -mountpoint /tmp/dwbc-mount -filename access.log -log /tmp/dwbc-events.jsonl
```

Stop with Ctrl-C or SIGTERM. Mounting hides any existing contents of the directory until unmounted. Do not mount over a real log directory. The log must be outside the mount directory. The default FUSE access policy permits the mounting user only; no `allow_other` option is enabled.

Reads generate bytes on demand and are logged as JSONL with time, caller PID/UID/GID, command and executable when accessible, path, offset and length. Writes, writable opens, attribute changes, file creation, removal, renaming and directory creation/removal handled by this prototype are rejected with EPERM and recorded as detections. Blocking temporary file creation also blocks common `sed -i` replacement workflows. The decoy does not mimic successful modification.

`go run ./cmd/ipv4slice -offset 0 -length 128` inspects generated data without mounting. Use `go test ./...` and `go test ./ipv4 -bench BenchmarkReadAt` to verify and measure the generator. The benchmark does not include FUSE, logging or kernel overhead.

## Limitations and operational risks

- This prototype has not been tested on a real Linux FUSE mount. Unsupported operations, such as links, are not all logged. A syscall rejected by the kernel before it reaches FUSE cannot be observed. Scans that exclude the mount or stop before reaching an address are not detected.
- Direct I/O reduces page-cache bypass of read callbacks, but FUSE does not provide a complete audit trail. A process may exit or its PID may be reused before `/proc` is read; command and executable fields are best-effort and may refer to a different process. Permissions and PID namespaces affect attribution. No process tree, command line or evidence dump is collected.
- Logs contain potentially sensitive process and filesystem data. Store them in a protected location, set retention and access controls, and review legal requirements. Existing log files retain their existing permissions. There is no rotation, redaction, queue, rate limit, external forwarding or action system; sustained reads can fill disk or block the filesystem. Read events are not necessarily malicious.
- There is no configuration file, overlay mode, event hooks or resource limits. An attacker with root or sufficient mount privileges can disable or bypass this service, change its logs or unmount it. Remote monitoring and independent operating system auditing are needed for stronger assurance.
- A full scan generates and transfers over 61 GB. Random offsets are supported. This synthetic file contains no real access history; an event alone does not prove compromise or C2 activity.

## Status

The generator has unit tests; Linux cross-compilation has been checked. Actual FUSE integration, `grep`, `sed -i`, `mv`, shutdown under load, and security review are outstanding. Do not deploy as a sole detection control.
