# ADR-0002: Use HTTP for Local Transfer

Status: Accepted

## Context

The receiving device must not require a qshare application.

Modern smartphones already provide a browser capable of downloading and uploading files through HTTP.

A custom transfer protocol would require a corresponding client implementation or deeper OS integration on the receiving device.

## Decision

qshare will use HTTP as the primary local transfer protocol.

The browser is the receiving-device client.

## Consequences

### Positive

* zero-install receiver;
* mature browser interoperability;
* straightforward streaming;
* native file download behavior;
* standard upload mechanisms;
* small implementation surface.

### Negative

* browser behavior varies by platform;
* HTTP on a local LAN does not provide transport confidentiality;
* some desirable browser capabilities require secure contexts;
* background-transfer control is limited.

## Alternatives considered

### Custom TCP protocol

Rejected because it requires a dedicated receiving client.

### WebRTC

Not selected for the initial design because connection establishment and signaling add complexity without improving the primary local QR workflow.

### Wi-Fi Direct transport

Not selected as the browser cannot serve as a general zero-install Wi-Fi Direct client.
