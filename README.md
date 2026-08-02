# NAT-Link

NAT-Link is a self-hosted NAT traversal system made of a Go server, a Go agent,
an embedded React dashboard, and a desktop agent. The implementation follows
`NAT-Link PRD.pdf` and targets Linux, Windows, and macOS.

## Development status

The repository is being built version by version:

- V1.0: authenticated agents and TCP tunnels
- V1.1: client/tunnel administration, traffic, and logs
- V1.2: Windows/macOS desktop agent and signed updates
- V2.0: QUIC, UDP, peer rendezvous, and relay nodes

See `docs/requirements.md` for the acceptance matrix.

