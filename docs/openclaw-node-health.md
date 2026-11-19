# Device health through diagnostics (issue #18)

## Collection and policy

The existing `echolocal.diagnostics.status` operation now returns a `health` snapshot
and, when compatible, `hostStats`. It remains in the diagnostics permission
group. The Node connection sends `node.event` / `node.host.stats` immediately
after connection and at most once per minute. Operator-only connections never
collect health metrics. No new permission, privilege escalation, shell command,
flash write or independent high-frequency polling is introduced.

The sender executes the same permission executor as an ordinary diagnostics
invocation. Revocation cancels active work, fences results and disconnects the
Node connection through the existing grant lifecycle. The sender checks the
policy generation again under the policy lock before transmission. Cleanup
clears the in-memory cache. An already in-flight delivery has a three-second
deadline; a completed revocation cannot be followed by a new delivery.

Collection is cached for at most 60 seconds, including diagnostic invocations.
The measurement time is `health.measuredAtMs` (Unix milliseconds), not the
time that a cached result was requested. Per-metric `available` booleans
distinguish unavailable values from genuine zero usage.

* CPU: device-wide aggregate /proc/stat delta over approximately one second;
  the actual interval is returned as intervalMs. Idle includes iowait;
  guest/guest_nice are not double counted. Busy/total is a 0–100 percentage
  across all CPUs, not a per-core sum and not load average. Counter resets,
  malformed input or inaccessible files produce unavailable, not zero.
* RAM: MemTotal and MemAvailable, converted from kernel KiB to bytes.
  Used = total - available, including reclaimable memory as available.
  No MemFree fallback is fabricated when MemAvailable is missing.
* Persistent storage: statfs of /data; tmpfs and ramfs are rejected.
  Available = f_bavail * block size; used = total - available (space not
  available to ordinary applications, including any filesystem reservation).
  No credential files or user content are read.
* Load average: optional kernel 1/5/15 minute values, explicitly separate
  from CPU utilization.

## Verified Gateway contract and current limitations

The deployed Gateway's canonical NodeHostStatsPayloadSchema is a closed object:
cpuCount; optional three-value loadAverage; mandatory memoryTotalBytes and
memoryFreeBytes; paired optional diskTotalBytes and diskAvailableBytes.
The event is node.host.stats, submitted through node.event with payloadJSON.
The Gateway adds updatedAtMs at receipt, broadcasts node.hostStats and projects
lastHostStats into the node inventory. Systems already renders RAM, disk and
**load**, not CPU percentage. EchoLocal maps available RAM to memoryFreeBytes,
matching the Gateway's available-memory meaning.

Unsupported fields must not be added to this payload: they invalidate the
entire event. If RAM is unavailable the entire compatible hostStats event is
omitted rather than reporting zero for mandatory memory fields. Disk and load
can be omitted independently.

**This is not full acceptance of issue #18 yet.** A coordinated Gateway/UI
change is required for CPU percentage, device measurement timestamps and
individual availability in the existing statistics presentation:

1. Extend NodeHostStatsPayloadSchema and saved snapshot validation to support
   optional cpuUsagePercent, cpuSampleIntervalMs, measuredAtMs and availability;
   permit unavailable RAM without inventing zero totals.
2. Extend the Systems telemetry model/cards with a separately labeled CPU %
   series; retain load average as load. Render per-field unavailable state.
3. Preserve measurement time rather than relabeling cached samples as current;
   suppress current-state presentation when disconnected or stale.
4. Add explicit snapshot invalidation on diagnostics revocation. The current
   protocol has no host-stats-clear event; an old server-side snapshot cannot
   be withdrawn by the device without a protocol change. No zero-valued
   invalidation is sent.

No Gateway source, configuration, permissions or UI bundle is modified by
this EchoLocal branch. Until that coordinated change is implemented and
visually verified, CPU percentage is available only in diagnostics and the
complete story must remain open. Hardware/build results belong in the issue
and PR; parser tests alone are not proof of device UI behavior.
