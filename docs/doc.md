# Sharpshooter — Full Project Documentation


---

## 1. Project Overview

### 1.1 Purpose

Sharpshooter is a reliable transport protocol library built on UDP, implemented in Go. It provides TCP-like connection-oriented semantics on top of UDP, without TCP's protocol fingerprint. This makes it suitable for bypassing protocol-based traffic detection and for use as a transport layer in P2P applications.

Core features:
- TCP-like 3-way handshake for connection establishment
- ACK-based retransmission for reliability
- Sliding window congestion control (auto scaling)
- Optional FEC (Forward Error Correction) via Reed-Solomon erasure coding
- Adaptive RTT/RTO calculation
- Health check and timeout detection
- Implements the `net.Conn` interface for seamless integration with Go's standard library

### 1.2 Tech Stack

| Technology | Version | Purpose |
|------------|---------|---------|
| Go | 1.15+ | Primary language |
| github.com/klauspost/reedsolomon | v1.9.9 | FEC erasure coding |

### 1.3 System Architecture

```
┌─────────────────────────────────────────────────────┐
│                 Application Layer                    │
│        Read/Write via net.Conn interface             │
└──────────────┬──────────────────────┬────────────────┘
               │                      │
       ┌───────▼───────┐      ┌───────▼───────┐
       │    Client      │      │    Server      │
       │  Dial() →      │      │  Listen() →   │
       │  *Sniper        │      │  *headquarters │
       │  (net.Conn)     │      │   Accept() →  │
       │                 │      │  *Sniper       │
       └───────┬───────┘      └───────┬───────┘
               │                      │
    ┌──────────▼──────────────────────▼──────────┐
    │           Core Transport (Sniper)            │
    │  ┌──────────┐ ┌──────────┐ ┌─────────────┐ │
    │  │  Sender   │ │ Receiver  │ │ Congestion   │ │
    │  │ Write()  │ │ Read()   │ │ Sliding Win  │ │
    │  │ shoot()  │ │ rcv()    │ │ RTT/RTO      │ │
    │  │ flush()  │ │ ack()    │ │ Scaling      │ │
    │  └──────────┘ └──────────┘ └─────────────┘ │
    │  ┌──────────┐ ┌──────────┐ ┌─────────────┐ │
    │  │   FEC     │ │ Handshake│ │  Health      │ │
    │  │ Reed-    │ │ 3-way    │ │ heartbeat    │ │
    │  │ Solomon  │ │ FSM      │ │ timeout      │ │
    │  └──────────┘ └──────────┘ └─────────────┘ │
    └──────────────────┬──────────────────────────┘
                       │
    ┌──────────────────▼──────────────────────────┐
    │            Protocol Layer                     │
    │   Ammo struct — Marshal/Unmarshal             │
    │   Format: | SIZE(4B) | SQE(4B) | CMD(2B) |   │
    │           | PROOF(4B) | BODY(...) |           │
    └──────────────────┬──────────────────────────┘
                       │
    ┌──────────────────▼──────────────────────────┐
    │               UDP (net.UDPConn)              │
    └─────────────────────────────────────────────┘
```

**Send data flow:**
1. Application calls `Write(data)`
2. Data is fragmented into Ammo packets of `packageSize` bytes
3. Ammo is placed in send window `ammoBag`, sent via `fire()` over UDP
4. Timer-driven retransmission via `autoShoot()` until ACKed
5. ACKed packets removed by `flush()`

**Receive data flow:**
1. UDP data parsed into Ammo via `Unmarshal`
2. Packets sorted by sequence into `rcvAmmoBag`
3. Consecutive packets merged into `rcvBuffer`
4. Application reads from `rcvBuffer` via `Read()`

---

## 2. Usage

### 2.1 Requirements

- Go 1.15 or later
- Dependency management: Go Modules

### 2.2 Configuration

No environment variables needed. All parameters are set via API calls.

### 2.3 Local Development

#### 2.3.1 Install

```bash
go get github.com/soyum2222/sharpshooter
```

#### 2.3.2 Run Examples

**Ping-Pong:**
```bash
# Terminal 1 — start server
cd example && go run pong.go

# Terminal 2 — start client
cd example && go run ping.go
```

**File transfer:**
```bash
# Start server to receive
cd example && go run sharp_transfer.go -l 8858 -o output.dat

# Start client to send
cd example && go run sharp_transfer.go -addr 127.0.0.1:8858 -i input.dat
```

**Simple file transfer:**
```bash
# Terminal 1 — receiver
cd example && go run client_receive.go

# Terminal 2 — sender
cd example && go run server_send.go
```

### 2.4 File Transfer Tool (sharp_transfer.go)

Command-line flags:

| Flag | Description | Example |
|------|-------------|---------|
| `-l` | Listen port (server mode) | `-l 8858` |
| `-addr` | Remote address (client mode) | `-addr 127.0.0.1:8858` |
| `-i` | Input file path (send mode) | `-i file.dat` |
| `-o` | Output file path (receive mode) | `-o output.dat` |
| `-c` | Resume transfer | `-c` |
| `-t` | Use TCP (for comparison) | `-t` |
| `-debug` | Enable debug output | `-debug` |

With `-c`, the tool checks if the output file exists and resumes from the end offset.

### 2.5 Testing

```bash
# Run all tests
go test ./...

# Run specific tests
go test -v -run TestDial
go test -v -run TestFecEncode ./...

# Benchmarks
go test -bench=BenchmarkMarshal ./protocol/
go test -bench=BenchmarkUnmarshal ./protocol/
```

### 2.6 Development Ports

| Service | Port | Description |
|---------|------|-------------|
| Default listen | 8858 | Ping-Pong and file transfer examples |
| Test server | 9090 | Unit tests |
| pprof (ping) | 18888 | Go profiling |
| pprof (pong) | 9999 | Go profiling |
| pprof (sharp_transfer) | 45671 | Go profiling |

---

## 3. Directory Structure & File Descriptions

### 3.1 Directory Tree

```
sharpshooter/
├── .gitignore
├── LICENSE
├── Readme.md
├── Readme_zh.md
├── go.mod
├── go.sum
├── closechan.go
├── dispatch.go
├── fec.go
├── fec_test.go
├── handshack.go
├── headquarters.go
├── read.go
├── receive.go
├── rtt.go
├── sharpshooter_test.go
├── sniper.go
├── wrap.go
├── protocol/
│   ├── protocol.go
│   └── protocol_test.go
├── tool/
│   ├── time_consuming.go
│   └── block/
│       ├── block.go
│       └── block_test.go
└── example/
    ├── ping.go
    ├── pong.go
    ├── server_send.go
    ├── client_receive.go
    └── sharp_transfer.go
```

### 3.2 Root Directory

| File | Description |
|------|-------------|
| `go.mod` | Go Modules definition, path `github.com/soyum2222/sharpshooter`, depends on `reedsolomon` |
| `go.sum` | Dependency checksums |
| `.gitignore` | Ignores `.idea` directory and `.exe` files |
| `LICENSE` | MIT License, Copyright 2021 soyum2222 |
| `Readme.md` | English README |
| `Readme_zh.md` | Chinese README |

### 3.3 Core Source Files (Root)

#### 3.3.1 `sniper.go`

Core file defining the `Sniper` struct and all its methods. Implements `net.Conn`.

**Key Sniper fields:**

| Field | Type | Description |
|-------|------|-------------|
| `packageSize` | `int64` | Max payload per packet, default 800 bytes |
| `rtt` / `rto` | `int64` | Round-trip time and retransmission timeout |
| `winSize` | `int32` | Send window size, default 64 |
| `sendId` | `uint32` | Send sequence counter |
| `rcvId` | `uint32` | Next expected receive sequence |
| `ammoBag` | `[]*protocol.Ammo` | Send window (unacknowledged packets) |
| `rcvAmmoBag` | `[]*protocol.Ammo` | Receive window (ordered) |
| `rcvBuffer` | `[][]byte` | Ordered receive data buffer |
| `sendBuffer` | `[]byte` | Send staging buffer for partial packets |
| `fec` | `bool` | Whether FEC is enabled |
| `ackCache` / `ackSendCache` | `[]uint32` | ACK buffer and send buffer |

**Statistics struct:** Traffic stats including TotalTraffic, EffectiveTraffic, TotalPacket, EffectivePacket, RTT, RTO, SendWin, ReceiveWin.

**Constants:**

| Constant | Value | Description |
|----------|-------|-------------|
| `DEFAULT_HEAD_SIZE` | 20 | Protocol header size (SIZE + SQE + CMD + PROOF) |
| `DEFAULT_INIT_SENDWIND` | 64 | Initial send window size |
| `DEFAULT_INIT_RECEWIND` | 1024 | Initial receive window size |
| `DEFAULT_INIT_PACKSIZE` | 800 | Default packet payload size |
| `DEFAULT_INIT_HEALTHTICKER` | 1 | Health check interval (seconds) |
| `DEFAULT_INIT_HEALTHCHECK_TIMEOUT_TRY_COUNT` | 10 | Max health check retries |
| `DEFAULT_INIT_HANDSHACK_TIMEOUT` | 6 | Handshake timeout retries |
| `DEFAULT_INIT_RTO_UNIT` | 200ms | Initial RTO baseline |
| `DEFAULT_INIT_DELAY_ACK` | 200ms | Delayed ACK time |
| `DEFAULT_INIT_INTERVAL` | 500 | Send interval |

**Connection states (STATUS_*):**
- `STATUS_NONE` (0) — Initial
- `STATUS_SECONDHANDSHACK` (1) — Sent 2nd handshake
- `STATUS_THIRDHANDSHACK` (2) — Sent 3rd handshake
- `STATUS_NORMAL` (3) — Normal communication
- `STATUS_CLOSEING1-3` (4-6) — Closing

**Key methods:**

| Method | Description |
|--------|-------------|
| `NewSniper(conn, aim)` | Create a new Sniper instance |
| `Write(b []byte)` | Send data (implements `net.Conn`), supports deadline |
| `Read(b []byte)` | Receive data (implements `net.Conn`), supports deadline and timeout |
| `Close()` | Close connection, send CLOSE packet, wait for peer confirmation |
| `LocalAddr()` / `RemoteAddr()` | Return local/remote address |
| `SetDeadline(t)` / `SetReadDeadline(t)` / `SetWriteDeadline(t)` | Set timeouts |
| `SetPackageSize(size)` | Set packet size |
| `SetRecWin(size)` / `SetSendWin(size)` | Set receive/send window size |
| `SetInterval(interval)` | Set send interval |
| `OpenFec(dataShards, parShards)` | Enable FEC mode |
| `OpenStaTraffic()` | Enable traffic statistics |
| `TrafficStatistics()` | Get traffic stats snapshot |
| `CleanStatistics()` | Reset statistics |
| `Debug()` | Enable debug logging |
| `shoot(put bool)` | Send engine, iterates ammoBag within window and fires |
| `fire(ammo)` | Send a single packet over UDP, record RTT sample timestamp |
| `flush()` | Remove ACKed packets from ammoBag |
| `autoShoot()` | Timer-driven retransmission trigger, registered in TimedSched |
| `ack(id)` | Add received packet ID to ACK buffer |
| `ackTimer()` | Periodic ACK sender, interval = min(rtt/4, 30ms) |
| `ackSender()` | Batch send ACKs from ackSendCache |
| `wrapACK()` / `unWrapACK()` | ACK compression/decompression (continuous ACKs as triplets) |
| `handleAck(ids)` | Process received ACKs, remove confirmed packets, trigger RTT calc |
| `expandWin()` / `zoomoutWin()` | Expand/shrink send window |
| `healthMonitor()` | Health check, close on timeout |
| `copyRcvBuffer(b)` | Copy data from receive buffer to user-provided buffer |
| `monitor()` | Standalone read goroutine (used without headquarters) |

**Send strategies:** `delaySend` (default) fragments by packet size; `fecSend` chunks data for FEC encoding.

#### 3.3.2 `dispatch.go`

Server-side core routing and connection management.

**headquarters struct:** UDP server manager.

| Field | Description |
|-------|-------------|
| `conn` | UDP connection |
| `Snipers` | `sync.Map` storing all active Sniper connections by address |
| `accept` | New connection channel, Accept() reads from here |
| `blockSign` | Read blocking signal |
| `errorSign` | Error signal |
| `closeSign` | Close signal |

**Key functions:**

| Function | Description |
|----------|-------------|
| `Dial(addr)` | Client connects, performs 3-way handshake, returns `net.Conn` |
| `Listen(addr)` | Server listens, returns `*headquarters` |
| `NewHeadquarters()` | Create headquarters instance |
| `Accept()` | Accept new connection (returns `net.Conn`) |
| `Close()` | Close server |
| `Addr()` | Return listen address |
| `monitor()` | Main read loop, receives UDP packets and routes them |
| `routing(sn, msg)` | Message routing by CMD type |
| `clear()` | Periodically clean up closed Sniper connections |
| `ReadFrom(b)` | Read data from any connection |
| `WriteToAddr(b, addr)` | Write to a specific address's connection |

#### 3.3.3 `handshack.go`

3-way handshake implementation.

| Function | Description |
|----------|-------------|
| `firstHandShack(h, remote)` | Handle 1st handshake (server), create new Sniper and reply 2nd handshake |
| `secondHandShack(h, remote, id)` | Handle 2nd handshake (client-side dial), reply 3rd handshake |
| `thirdHandShack(h, remote, id)` | Handle 3rd handshake (server), verify ID and put connection into accept channel |

Handshake flow:
```
Client                          Server
  |---- FIRSTHANDSHACK ---------->|
  |<--- SECONDHANDSHACK ----------|  (with random ID)
  |---- THIRDHANDSHACK ---------->|  (echo ID for verification)
  |         Connected              |
```

#### 3.3.4 `receive.go`

Packet reception and ordering.

| Function | Description |
|----------|-------------|
| `rcvnoml(ammo)` | Normal mode: sort by sequence, merge consecutive packets into rcvBuffer |
| `rcvfec(ammo)` | FEC mode: collect a full shard group, decode, recover original data |

Both modes use `int(ammo.Id) - int(s.rcvId)` for bagIndex to handle sequence number wraparound.

#### 3.3.5 `wrap.go`

Data packaging for sending.

| Function | Description |
|----------|-------------|
| `wrapnoml()` | Normal mode: fragment sendBuffer into Ammo packets by packageSize |
| `wrapfec()` | FEC mode: chunk sendBuffer by `dataShards * packageSize`, encode as data + parity shards |

#### 3.3.6 `read.go`

`Sniper.Read()` implementation. Blocks waiting for data arrival. Supports deadline and timeout mechanisms. Uses `readBlock` channel to notify new data, `closeChan` to notify connection closure.

#### 3.3.7 `fec.go`

FEC (Forward Error Correction) codec based on Reed-Solomon algorithm.

| Type | Description |
|------|-------------|
| `fecEncoder` | Encoder with dataShards, parShards and reedsolomon.Encoder |
| `fecDecoder` | Decoder with dataShards, parShards and reedsolomon.Encoder |

| Method | Description |
|--------|-------------|
| `newFecEncoder(data, par)` | Create encoder |
| `newFecDecoder(data, par)` | Create decoder |
| `encode(b)` | Encode: prepend 4-byte length, split into shards + parity |
| `decode(b)` | Decode: Reconstruct missing shards, Join original data |
| `estimatedLength(length)` | Estimate total FEC-encoded length |

#### 3.3.8 `rtt.go`

RTT/RTO calculation.

`calrto(rtt)` — Update SRTT (smoothed RTT) and RTO (retransmission timeout) based on latest RTT sample:
- First sample: `SRTT = rtt`, `RTO = rtt * 2`
- Subsequent: `SRTT = 0.3 * rtt + 0.7 * SRTT`, `RTO = SRTT * 1.5`
- Outlier filter: ignore rtt > 2 * current_rtt

#### 3.3.9 `closechan.go`

Safe channel closing utility. Prevents panic from double-close.

`chanCloser` struct uses a mutex to protect `close` operations, checking via select if the channel is already closed.

#### 3.3.10 `headquarters.go`

Timed scheduler system, inspired by kcp-go.

`TimedSched` — Heap-based global timed task scheduler with CPU-count parallel workers.

| Method | Description |
|--------|-------------|
| `NewTimedSched(parallel)` | Create scheduler, launch `parallel` worker goroutines |
| `Put(f, deadline)` | Submit a timed task |
| `Close()` | Close the scheduler |

`SystemTimedSched` — Global singleton created at library load.

#### 3.3.11 `sharpshooter_test.go`

Core test file covering:

| Test | Description |
|------|-------------|
| `TestCopyRcvBuffer` | Test receive buffer copy logic |
| `TestDial` | Test Dial/Accept connection |
| `TestSniper_Close` | Test connection close (both sides) |
| `TestSniper_Close2` | Test close after sending |
| `TestSniper_ClientClose` | Test client-initiated close |
| `TestSniper_ServerClose` | Test server-initiated close |
| `TestSniper_WriteCloseConn` | Test write after close returns error |
| `TestSniper_ReadCloseConn` | Test read after close returns error |
| `TestSniper_SetDeadline` | Test general deadline |
| `TestSniper_SetReadDeadline` | Test read deadline |
| `TestSniper_SetWriteDeadline` | Test write deadline |
| `Test_CreateLargeConnection` | Test 1024 concurrent connections |
| `TestUnWrapACK` | Test ACK decompression |
| `TestSendBigData` | Test sending 1024 x 1MB data blocks |

All connection tests enable FEC (4,3).

#### 3.3.12 `fec_test.go`

FEC codec test — `TestFecEncode`: encode 16 bytes into 6 shards (4+2), drop 2 shards, decode to recover.

### 3.4 protocol/ Package

#### 3.4.1 `protocol.go`

Protocol layer implementation. Defines packet format and serialization.

**Ammo struct (packet):**

| Field | Type | Size | Description |
|-------|------|------|-------------|
| `Length` | `uint32` | 4 bytes | Total size of Body + 10 (excludes Length field itself) |
| `Id` | `uint32` | 4 bytes | Sequence number |
| `Kind` | `uint16` | 2 bytes | Packet type (CMD) |
| `proof` | `uint32` | 4 bytes | Checksum (count of set bits in Body) |
| `Body` | `[]byte` | variable | Payload data |

**CMD types:**

| Value | Constant | Description |
|-------|----------|-------------|
| 0 | `ACK` | Acknowledgment |
| 1 | `NORMAL` | Data |
| 2 | `FIRSTHANDSHACK` | 1st handshake |
| 3 | `SECONDHANDSHACK` | 2nd handshake |
| 4 | `THIRDHANDSHACK` | 3rd handshake |
| 5 | `CLOSE` | Close connection (FIN) |
| 6 | `CLOSERESP` | Close response |
| 7 | `HEALTHCHECK` | Health check |
| 8 | `HEALTCHRESP` | Health check response |
| 9 | `NORMALTAIL` | Last data packet + close marker |
| 10 | `OUTOFAMMO` | Reserved |

**Checksum:** The `proof` field stores the total count of set bits (1s) across all bytes in Body using a precomputed lookup table `table`. Computed during `Marshal`, verified during `Unmarshal`.

**Key functions:**

| Function | Description |
|----------|-------------|
| `Marshal(ammo)` | Serialize Ammo to byte stream |
| `Unmarshal(b)` | Deserialize byte stream to Ammo, verify length and proof |
| `Free()` | Reset all Ammo fields, return to pool |

**Ammo helpers:**
- `AckAdd()` — ACK count +1
- `ShootAdd()` / `ShootCount()` — Send count increment/read

#### 3.4.2 `protocol_test.go`

Protocol tests:

| Test | Description |
|------|-------------|
| `TestMarshalUnmarshal` | Serialize/deserialize round-trip test |
| `TestRogue` | Malformed packet detection (wrong length) |
| `BenchmarkMarshal` | Serialization benchmark |
| `BenchmarkUnmarshal` | Deserialization benchmark |

### 3.5 tool/ Package

#### 3.5.1 `tool/time_consuming.go`

Performance timing utility.

`TimeConsuming()` — Returns a defer function that measures execution time. Uses `runtime.Caller(1)` to get the caller's function name and prints elapsed time (nanoseconds).

Usage:
```go
defer tool.TimeConsuming()()
```

#### 3.5.2 `tool/block/block.go`

Goroutine block/wake primitive.

**Blocker struct:**

| Method | Description |
|--------|-------------|
| `NewBlocker()` | Create new Blocker |
| `Block()` | Block current goroutine until `Pass()` or `Close()` |
| `Pass()` | Wake one blocked goroutine |
| `PassBT(duration)` | Try to wake within duration, return on timeout |
| `Select()` | Return a channel for use in select |
| `Close()` | Wake all blocked goroutines and permanently close Blocker |

Used in Sniper for Write flow control: `Block()` when window is full, `Pass()` when space opens.

#### 3.5.3 `tool/block/block_test.go`

Blocker tests: single block/wake, multi-goroutine block/wake, ordering verification, Close wakes all.

### 3.6 example/ Directory

| File | Description |
|------|-------------|
| `ping.go` | Client example, connects with FEC(10,3), sends "ping" and receives "pong", with pprof |
| `pong.go` | Server example, listens with FEC(10,3), receives "ping" and replies "pong", with pprof |
| `server_send.go` | File send server, listens and sends `./test` via `io.Copy` |
| `client_receive.go` | File receive client, connects and reads data in a loop (discards) |
| `sharp_transfer.go` | Full file transfer tool with CLI flags, resume, TCP comparison, progress bar, speed display, debug mode |

### 3.7 image/ Directory

| File | Description |
|------|-------------|
| `network.png` | Transfer speed screenshot |
| `network-utilization.png` | Network utilization screenshot |

---

## 4. Protocol Format

### 4.1 Packet Format

```
| SIZE(4byte) | SQE(4byte) | CMD(2byte) | PROOF(4byte) | CONTENT(.......) |
```

| Field | Size | Description |
|-------|------|-------------|
| SIZE | 4 bytes | Total bytes of SQE + CMD + PROOF + CONTENT (excludes itself) |
| SQE | 4 bytes | Sequence number, consecutive packets have consecutive SQE |
| CMD | 2 bytes | Packet type |
| PROOF | 4 bytes | Checksum, count of set bits in Body |
| CONTENT | variable | Payload |

Max packet length is `DEFAULT_INIT_PACKSIZE` (800) or user-configured `packageSize`.

### 4.2 ACK Packet Format

```
| SIZE(4byte) | SQE(4byte) | CMD(2byte) | ackSQE1(4byte)| ackSQE2(4byte) | ackSQE3(4byte) | ... |
```

**ACK compression rules:**
- Fewer than 3 consecutive ACKs: listed individually
- 3 or more consecutive ACKs: represented as triplet `(start, start, end)`
  - e.g. `|5|5|10|` means ACK 5 through 10

### 4.3 Connection State Machine

```
STATUS_NONE ─────────────────────┐
    │ firstHandShack (server)     │ secondHandShack (client-side)
    ▼                             ▼
STATUS_SECONDHANDSHACK    STATUS_THIRDHANDSHACK
    │ thirdHandShack (server)     │ (transient)
    ▼
STATUS_NORMAL ──── Data Transfer ────► STATUS_NORMAL
    │
    │ Close()
    ▼
STATUS_CLOSEING → Connection closed
```

---

## 5. Key Business Logic

### 5.1 Connection Establishment (3-way Handshake)

1. **Client → Server:** `FIRSTHANDSHACK` — client initiates connection
2. **Server → Client:** `SECONDHANDSHACK` — server creates Sniper, generates random ID, replies
3. **Client → Server:** `THIRDHANDSHACK` — client echoes ID, server verifies
4. Connection established, data transfer begins

Each handshake step has state verification to prevent malicious/duplicate packets.

### 5.2 Data Send & Retransmission

**Send flow:**
1. Application calls `Write(data)`
2. Data staged in `sendBuffer`, fragmented into Ammo by `packageSize`
3. Ammo placed in send window `ammoBag`, sent via `fire()`
4. `autoShoot()` timer periodically triggers retransmission (interval = min(RTO, interval))
5. ACKed packets removed by `handleAck()`
6. `flush()` compacts ammoBag removing leading nil gaps
7. When window has space, `writerBlocker.Pass()` wakes blocked Write

**Delayed ACK:**
- ACKs buffered in `ackCache`, periodically batch-sent by `ackTimer()`
- Send interval: `min(rtt/4, 30ms)`
- Immediate send when ACK buffer exceeds `packageSize/4`
- Continuous ACKs compressed as triplets to save bandwidth

### 5.3 Sliding Window Congestion Control

- **Expand:** When cycle send volume/window > 0.9 and low loss, `winSize *= 1.25`, max 32768
- **Shrink:** When loss rate > 50%, `winSize /= 1.25`, min `minSize` (default 64)

### 5.4 RTT/RTO Calculation

- Sample RTT from first packet in each cycle
- First sample: `SRTT = rtt`, `RTO = rtt * 2`
- Subsequent: `SRTT = 0.3 * newRTT + 0.7 * SRTT`, `RTO = SRTT * 1.5`
- Outlier filter: ignore RTT > 2 * current SRTT

### 5.5 FEC Forward Error Correction

Using Reed-Solomon erasure coding:
- Data split into `dataShards` shards, plus `parShards` parity shards
- Receiver only needs `dataShards` shards to recover original data
- Tolerates up to `parShards` shard losses
- Default example: dataShards=10, parShards=3 (tolerates 30% loss)

### 5.6 Health Check

- Send `HEALTHCHECK` packet every 1 second
- Peer replies with `HEALTCHRESP`
- 10 consecutive failures triggers connection teardown
- Closes all related channels and connection

### 5.7 Connection Close

1. Initiator calls `Close()`
2. Waits for all packets in ammoBag to be sent
3. Sends `CLOSE` packet
4. Receiver marks last data packet as `NORMALTAIL`
5. Receiver sends `CLOSERESP` after remaining data is sent
6. Both sides close the connection

---

## 6. Notes

### 6.1 Development Conventions

- Implements Go's `net.Conn` interface, can replace TCP directly
- Uses `sync.Pool` to reuse Ammo objects, reducing GC pressure
- Global timed scheduler `SystemTimedSched` uses CPU-count parallel workers

### 6.2 Known Limitations

- Go version requires 1.15+ (outdated)
- FEC encoding/decoding has CPU overhead
- Sequence numbers use `uint32`, very long connections may wrap (handled via int32 delta)
- `rand.Seed` called in `handshack.go` `init()`, deprecated in Go 1.20+
- Original README had incorrect example links (both pointed to same file)

### 6.3 Security Notes

- Protocol checksum uses simple bit counting, not cryptographic
- No encryption, data transmitted in plaintext
- No authentication, suitable for trusted networks or VPNs
- Suitable for bypassing protocol fingerprinting (not content inspection)

### 6.4 Operations Notes

- Examples include built-in `net/http/pprof` for performance profiling
- Network utilization depends on send window size and network conditions
- Use `OpenStaTraffic()` to enable traffic statistics for monitoring

---

## 7. Dependencies

### 7.1 Direct Dependencies

| Dependency | Version | Purpose |
|------------|---------|---------|
| `github.com/klauspost/reedsolomon` | v1.9.9 | Reed-Solomon erasure coding for FEC |

### 7.2 Standard Library Usage

| Package | Purpose |
|---------|---------|
| `net` | UDP networking |
| `sync` / `sync/atomic` | Concurrency control |
| `container/heap` | Timed scheduler heap |
| `encoding/binary` | Big-endian byte encoding |
| `time` | Timers, deadlines |
| `math` | Math calculations |
| `sort` | ACK sorting |
| `runtime` | CPU count |
| `errors` | Error definitions |
| `fmt` | Debug output |
| `io` | io.ReadFull / io.Copy |
| `os` | File operations |
| `testing` | Unit testing |
