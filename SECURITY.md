# Security and threat model

This filesystem records access to synthetic data and rejects mutations. It does
not establish that a reader is malicious, prevent attacks elsewhere, or survive
host-root compromise. Deploy in a dedicated empty mount directory. Keep its
configuration, log parent directory, evidence directory and hook executables
owned by a trusted administrator and not writable by monitored users.

Procfs metadata is best effort. A start-time comparison detects many exit/reuse
races, but is not an atomic process snapshot and cannot guarantee identity across
PID namespaces or exec. Missing information is retained as an error. Arguments
are fully masked; environment variables and process memory are never collected.
Command names, executable paths, event paths and UIDs may still be sensitive.

Evidence collection is opt-in and bounded by file count, tree depth (8), metadata
read size (8 KiB per read) and event history count. Existing directory entries
count toward the evidence limit. Use a private dedicated directory. Log rollover
truncates old data. Queue overflow drops events with a stderr diagnostic. These
limits prevent unbounded local growth but are not lossless auditing. Configure a
trusted hook to forward events to independently controlled retention if needed.

Hooks receive JSON on stdin without a shell. They execute with the daemon's
privileges; allowlisting is not isolation. Only trusted administrators should
configure them. Hook children must be managed by the hook itself. No default
hook or outbound network connection is enabled.

For suspected defects, avoid posting real logs, arguments or host paths in public
issues. Report a minimal synthetic reproducer through a private channel agreed
with the repository maintainer.
