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

CPU utilization, RAM, disk and original kernel load are cached for at most
60 seconds, including diagnostic invocations. CPU-demand load has its own
in-memory five-second sampler and measurement timestamp (see below).
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
* Original Linux load: optional kernel 1/5/15 minute values remain in
  health.loadAverage for diagnostics only, explicitly separate from both
  CPU utilization and the CPU-demand estimate below.

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
The current protocol has no host-stats-clear event. However, the existing
grant lifecycle reconnects the Node: the real-device test confirmed that
this removes current hostStats from the Gateway inventory. They remained
absent for more than one update interval with diagnostics disabled, while
audio RPCs and connectivity remained functional. Re-enabling diagnostics
restored collection and publication. Historical/offline UI presentation still
needs visual verification; no zero-valued invalidation is sent.

No Gateway source, configuration, permissions or UI bundle is modified by
this EchoLocal branch. Until that coordinated change is implemented and
visually verified, CPU percentage is available only in diagnostics and the
complete story must remain open. Hardware/build results belong in the issue
and PR; parser tests alone are not proof of device UI behavior.


## Hardware verification: 2026-11-19

Tested application commit: 04a0e367d474d975d12fc50003549115c509406c.
Version: 0.0.15-dev.issue18.04a0e36.
Athena binary SHA-256:
9f077d604167b09560337095c4381d90d7fe60143e48188a8ae98376fcd42854.
Build and installed binary hashes matched; service was running and screen ready.
Installation used the normal installer without reboot; settings were retained.

The health test binary passed all five tests directly on Athena. The installed
service returned CPU percentage with a 1001 ms sample across 8 CPUs, RAM total
2021113856 bytes and data filesystem total 3631857664 bytes. Live available/used
values were plausible. The Gateway inventory contained matching hostStats,
proving the event was accepted and integrated, not only a raw diagnostic result.
Diagnostics-off denied the command and removed it from advertised commands.
After more than 60 seconds, hostStats remained absent; audio status still worked.
Diagnostics-on restored readings and the original grants.

OpenClaw/Talk tests and vet passed, as did the Athena application build.
Parser/error/cancellation tests cover unavailable inputs, counter reset,
normalization, cache/reset and revocation fencing. Full spoken audio interaction,
physical touch and visual Gateway statistics/offline presentation are not claimed.
Browser verification was blocked by an existing Chromium profile lock.
The CPU percentage/availability/timestamp Gateway/UI extension remains outstanding.

The previous binary's verified hash is:
45edfc3dca0a1777703f1451a57f8a4c77e6241e1093e7e22d8b3020043fe95f.
A device-local binary rollback was retained; reinstall it through the normal
installer, preserving settings. Exact private paths are kept in operational notes.


## Owner-requested CPU-demand load for the existing UI

The owner explicitly requested retaining the existing load indicator while
excluding Fire OS D-state kernel/driver sleepers from its CPU-pressure signal.
The unchanged Gateway schema/UI interprets loadAverage[0] / cpuCount as a
fraction, warns at 70%, and marks danger at 100%. Kernel load near 17 on this
8-CPU device therefore appears saturated despite much lower measured CPU usage.

For EchoLocal, **hostStats.loadAverage now carries an estimated runnable-only
CPU-demand load**, NOT standard Linux loadavg and NOT CPU percentage. This is
an explicitly requested compatibility projection; consumers expecting canonical
Linux load must use health.loadAverage instead. No core-count inflation or
subtraction of a guessed fixed D-thread baseline is used.

The collector reads only /proc/stat:procs_running, the system-wide scheduler
count of running/runnable tasks, at most once every five seconds. One current
observer thread is discounted, with the result clamped at zero. No per-task
enumeration, command-line reads, subprocesses or flash writes are needed.
D-state tasks are not included by this kernel counter.

For each five-second snapshot x and actual elapsed monotonic time dt, the
1/5/15-minute series is updated as:

    average += (x - average) * (1 - exp(-dt / tau))
    tau = 60, 300, 900 seconds

These are sampled exponential averages, not exact rolling arithmetic windows.
Short CPU bursts between samples can be missed. A load of 8 represents mean
sampled demand for 8 CPUs, not necessarily exactly 100% busy time. The already
available cpu.percent remains the measured CPU-time utilization.

health.cpuLoad includes availability, the three averages, current runnable
count, source, sampledAtMs, sampleIntervalMs, observedMs and warmingUp.
A new connection/history is seeded from its first actual sample (never a
fabricated healthy zero), and is marked warmingUp for the first 15 minutes.
Missing/malformed counters invalidate history and omit the compatible
loadAverage field; a subsequent valid sample reseeds it. Gaps over 30 seconds
also reseed. No stale historical estimate is silently reused after reconnect
or permission edits.

The connected Node runs the existing diagnostics executor every five seconds.
The heavy one-second CPU-time probe and RAM/disk/kernel-load reads retain their
one-minute cache; host-stats transmission remains at most once per minute.
Denied diagnostics never reaches the collector. Existing cancellation,
policy-generation fences and disconnect cleanup stop/reset the sampler.

The current UI cannot label this custom source or its warm-up state. That
limitation and the compatibility meaning must remain documented; a future
Gateway/UI extension should distinguish CPU-demand load, original kernel
load and CPU utilization explicitly. This projection does not implement the
outstanding native CPU-percent/availability/timestamp UI extension.
