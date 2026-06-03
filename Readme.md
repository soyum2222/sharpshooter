<h1 align="center">Sharpshooter</h1>

<p align="center">
  <b>Reliable UDP transport protocol for Go</b>
</p>

<p align="center">
  <img src="https://img.shields.io/github/license/soyum2222/sharpshooter?logo=Github&style=flat-square" />
  <img src="https://img.shields.io/github/go-mod/go-version/soyum2222/sharpshooter?logo=Go&style=flat-square" />
  <img src="https://img.shields.io/github/v/tag/soyum2222/sharpshooter?label=version&style=flat-square" />
  <img src="https://img.shields.io/github/commit-activity/m/soyum2222/sharpshooter?logo=Github&style=flat-square" />
</p>

<p align="center">
  <a href="Readme_zh.md">中文文档</a>
</p>

---

Sharpshooter is a reliable transport protocol built on UDP, implemented in Go. It provides TCP-like connection-oriented semantics without TCP's protocol fingerprint, making it suitable for bypassing protocol-based traffic detection and serving as a transport layer for P2P applications.

**Features:**

- TCP-like 3-way handshake
- ACK-based retransmission
- Adaptive sliding window congestion control
- Optional FEC (Forward Error Correction) via Reed-Solomon
- RTT/RTO auto-calibration
- Health check & timeout detection
- Implements `net.Conn` interface
- Zero external dependencies besides Reed-Solomon

---

## Quick Start

```bash
go get github.com/soyum2222/sharpshooter
```

### Server

```go
l, _ := sharpshooter.Listen(":8858")
conn, _ := l.Accept()
// conn implements net.Conn — use Read/Write directly
```

### Client

```go
conn, _ := sharpshooter.Dial("127.0.0.1:8858")
// Enable FEC (optional)
conn.(*sharpshooter.Sniper).OpenFec(10, 3)
conn.Write([]byte("hello"))
```

More examples in the [`example/`](https://github.com/soyum2222/sharpshooter/tree/master/example) directory.

---

## API

| Method | Description |
|--------|-------------|
| `Dial(addr) (net.Conn, error)` | Connect to a remote listener |
| `Listen(addr) (*headquarters, error)` | Start a UDP listener |
| `Accept() (net.Conn, error)` | Accept a new connection |
| `OpenFec(data, par)` | Enable FEC (e.g. `OpenFec(10, 3)` tolerates 30% loss) |
| `SetPackageSize(size)` | Set packet payload size |
| `SetSendWin(size)` / `SetRecWin(size)` | Set send/receive window |
| `OpenStaTraffic()` | Enable traffic statistics |
| `TrafficStatistics()` | Get traffic stats snapshot |

All standard `net.Conn` methods (`Read`, `Write`, `Close`, `SetDeadline`, etc.) are supported.

For protocol specification and detailed documentation, see [`docs/`](docs/).

---

## File Transfer Tool

```bash
# Server (receive)
go run example/sharp_transfer.go -l 8858 -o output.dat

# Client (send)
go run example/sharp_transfer.go -addr 127.0.0.1:8858 -i input.dat

# Resume transfer
go run example/sharp_transfer.go -addr 127.0.0.1:8858 -i input.dat -o output.dat -c

# Compare with TCP
go run example/sharp_transfer.go -addr 127.0.0.1:8858 -i input.dat -t
```

---

## Related

- [sharpshooter-tunel](https://github.com/soyum2222/sharpshooter-tunel) — TCP ↔ Sharpshooter bridge

## License

[MIT](LICENSE)
