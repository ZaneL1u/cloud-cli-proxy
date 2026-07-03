---
status: resolved
---

# Debug: galactus managed-user DNS loop and resource exhaustion

## Symptoms

- Managed-user `sing-box` consumed multiple CPU cores and pushed the host into sustained high temperature.
- Control plane repeatedly logged Docker network connect failures against the compose network.
- Control plane healthcheck was unhealthy when `CONTROL_PLANE_ADDR` was overridden away from `:8080`.
- Host creation was blocked by runtime egress verification even when the container itself was usable.

## Root Causes

1. The local DNS stub used a `direct` inbound on `127.0.0.1:53`, but route matching could fall through to the private-IP direct rule. That allowed `sing-box` to connect back to `127.0.0.1:53`, creating a DNS loop.
2. The container tun address was hard-coded to `172.19.0.1/30`, which overlaps common Docker compose networks such as `172.19.0.0/16`.
3. Host creation treated egress IP verification as a hard gate. A proxy outage or mismatch could fail an otherwise started fail-closed container.
4. Zero resource limits were interpreted as unlimited, allowing a single container process to consume host-wide CPU.
5. Compose healthcheck was pinned to port 8080 instead of deriving the port from `CONTROL_PLANE_ADDR`.

## Fix Plan

1. Add regression tests for DNS stub routing, tun CIDR selection, create-time verifier behavior, and resource defaulting.
2. Change DNS stub route so stub traffic is always hijacked before any private/direct route can match.
3. Move the default tun CIDR away from Docker private/compose ranges.
4. Make `PrepareHost` non-blocking and skip create-time egress IP verification.
5. Interpret zero resource limits as safe defaults, not unlimited.
6. Make compose healthcheck follow `CONTROL_PLANE_ADDR`.

## Resolution

- Code now renders the DNS stub as `dns-stub` and hijacks stub traffic before private/direct routes, removing the `127.0.0.1:53` loop path.
- Container tun CIDR now uses `198.18.0.1/30`, avoiding Docker bridge/compose private CIDR overlap.
- Host creation no longer fails on create-time egress IP verification; the container entrypoint remains fail-closed for sing-box and nft setup.
- Zero resource values now resolve to safe defaults: 4096 MB memory, 2 CPU, and 1024 PIDs. Docker create/update also sets `memory-swap` equal to memory.
- Compose healthcheck now derives the port from `CONTROL_PLANE_ADDR`.
- galactus was hotfixed with a local control-plane image tag, the existing user container config was updated and restarted, and the existing container now has CPU/memory/PID limits.
