# ADR-0003: Use a Temporary SoftAP for Direct Mode

Status: Accepted in principle

## Context

qshare must eventually work when:

* no existing LAN is available;
* both the PC and smartphone have Wi-Fi;
* the smartphone has no qshare application.

Wi-Fi Direct is attractive technically but does not provide a sufficiently general browser-only receiving workflow.

Requiring the user to enable phone tethering adds manual setup and reverses the desired interaction.

## Decision

Direct Mode will use a temporary Wi-Fi access point created by the computer where the operating system and hardware permit it.

The computer will provide the local network services required for the browser to reach qshare.

Possible supporting components include:

* SoftAP;
* DHCP;
* local DNS;
* captive portal handling.

Exact platform implementations remain separate from core transfer logic.

## Consequences

### Positive

* no existing infrastructure is required;
* receiving device remains zero-install;
* browser-based transfer remains unchanged;
* network bootstrap can be represented by standard Wi-Fi QR mechanisms.

### Negative

* hotspot APIs differ substantially between operating systems;
* some Wi-Fi hardware or drivers may not support the required mode;
* captive portal behavior varies across Android and iOS;
* privileged networking operations may require OS-specific authorization;
* implementation and testing are significantly more difficult than LAN Mode.

## Implementation constraint

Direct Mode must not contaminate core transfer code with OS-specific networking logic.

Platform networking must remain behind an adapter boundary.

## Open questions

Before Direct Mode is considered stable, determine:

* supported Linux network managers;
* privilege model;
* DHCP implementation strategy;
* DNS requirements;
* captive portal behavior on current Android and iOS versions;
* whether active Internet connectivity should be preserved when technically possible;
* behavior when hotspot creation fails.
