# Sharpshooter — 完整项目文档


---

## 1. 项目概述

### 1.1 项目作用

Sharpshooter 是一个基于 UDP 的可靠传输协议库，使用 Go 语言实现。它提供类似 TCP 的面向连接的语义，但建立在 UDP 之上，因此没有 TCP 的协议特征。这使得它可以用于绕过基于协议特征的检测，也可作为 P2P 应用的底层传输协议。

核心特性包括：
- 三次握手建立连接（类似 TCP）
- ACK 确认与重传机制保证可靠性
- 滑动窗口拥塞控制（自动扩缩窗口）
- 可选的 FEC（前向纠错）冗余编码，恢复丢包
- RTT/RTO 自适应计算
- 健康检查与超时断连
- 实现了 `net.Conn` 接口，可直接与 Go 标准库配合使用

### 1.2 技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| Go | 1.15+ | 主语言 |
| github.com/klauspost/reedsolomon | v1.9.9 | FEC 纠删码编解码 |

### 1.3 系统架构

```
┌─────────────────────────────────────────────────────┐
│                   应用层 (Application)                │
│         使用 net.Conn 接口读写数据                     │
└──────────────┬──────────────────────┬────────────────┘
               │                      │
       ┌───────▼───────┐      ┌───────▼───────┐
       │  Client 端     │      │  Server 端     │
       │  Dial() →     │      │  Listen() →   │
       │  *Sniper       │      │  *headquarters │
       │ (net.Conn)     │      │   Accept() →  │
       │                │      │  *Sniper       │
       └───────┬───────┘      └───────┬───────┘
               │                      │
    ┌──────────▼──────────────────────▼──────────┐
    │              核心传输层 (Sniper)              │
    │  ┌──────────┐ ┌──────────┐ ┌─────────────┐ │
    │  │ 发送引擎  │ │ 接收引擎  │ │  拥塞控制    │ │
    │  │ Write()  │ │ Read()   │ │ 滑动窗口     │ │
    │  │ shoot()  │ │ rcv()    │ │ RTT/RTO     │ │
    │  │ flush()  │ │ ack()    │ │ 伸缩策略     │ │
    │  └──────────┘ └──────────┘ └─────────────┘ │
    │  ┌──────────┐ ┌──────────┐ ┌─────────────┐ │
    │  │  FEC编码  │ │  握手机制  │ │  健康检查    │ │
    │  │ Reed-    │ │ 三次握手  │ │ heartbeat   │ │
    │  │ Solomon  │ │ 状态机    │ │ timeout     │ │
    │  └──────────┘ └──────────┘ └─────────────┘ │
    └──────────────────┬──────────────────────────┘
                       │
    ┌──────────────────▼──────────────────────────┐
    │           协议层 (protocol 包)                │
    │   Ammo 结构体 — Marshal/Unmarshal            │
    │   包格式: | SIZE(4B) | SQE(4B) | CMD(2B) |  │
    │           | PROOF(4B) | BODY(...) |          │
    └──────────────────┬──────────────────────────┘
                       │
    ┌──────────────────▼──────────────────────────┐
    │                UDP (net.UDPConn)             │
    └─────────────────────────────────────────────┘
```

**数据流向（发送端）：**
1. 应用调用 `Write(data)`
2. 数据被分片为 `packageSize` 大小的 Ammo 包
3. Ammo 放入发送窗口 `ammoBag`，通过 `fire()` 经 UDP 发出
4. 启动定时重传 `autoShoot()`，直到收到 ACK
5. 收到 ACK 后 `flush()` 移除已确认的包

**数据流向（接收端）：**
1. UDP 收到数据，`Unmarshal` 解析为 Ammo
2. 按 sequence 排序放入 `rcvAmmoBag`
3. 连续的包被合并到 `rcvBuffer`
4. 应用调用 `Read()` 从 `rcvBuffer` 获取数据

---

## 2. 使用方式

### 2.1 环境要求

- Go 1.15 或更高版本
- 依赖管理：Go Modules

### 2.2 配置说明

无需环境变量配置。所有参数通过 API 调用设置。

### 2.3 本地开发

#### 2.3.1 获取依赖

```bash
go get github.com/soyum2222/sharpshooter
```

#### 2.3.2 运行示例

**Ping-Pong 示例：**
```bash
# 终端 1 — 启动服务端
cd example && go run pong.go

# 终端 2 — 启动客户端
cd example && go run ping.go
```

**文件传输示例：**
```bash
# 启动服务端监听并接收文件
cd example && go run sharp_transfer.go -l 8858 -o output.dat

# 启动客户端发送文件
cd example && go run sharp_transfer.go -addr 127.0.0.1:8858 -i input.dat
```

**简单文件传输示例：**
```bash
# 终端 1 — 接收端
cd example && go run client_receive.go

# 终端 2 — 发送端
cd example && go run server_send.go
```

### 2.4 文件传输工具 (sharp_transfer.go)

支持命令行参数：

| 参数 | 说明 | 示例 |
|------|------|------|
| `-l` | 监听端口（服务端模式） | `-l 8858` |
| `-addr` | 远程地址（客户端模式） | `-addr 127.0.0.1:8858` |
| `-i` | 输入文件路径（发送模式） | `-i file.dat` |
| `-o` | 输出文件路径（接收模式） | `-o output.dat` |
| `-c` | 断点续传 | `-c` |
| `-t` | 使用 TCP（对比测试） | `-t` |
| `-debug` | 开启调试输出 | `-debug` |

断点续传 `-c` 会先检查输出文件是否存在，如果存在则从文件末尾偏移量开始续传。

### 2.5 测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test -v -run TestDial
go test -v -run TestFecEncode ./...

# 基准测试
go test -bench=BenchmarkMarshal ./protocol/
go test -bench=BenchmarkUnmarshal ./protocol/
```

### 2.6 开发端口

| 服务 | 端口 | 说明 |
|------|------|------|
| 默认监听端口 | 8858 | Ping-Pong 和文件传输示例 |
| 测试服务端口 | 9090 | 单元测试用 |
| pprof (ping) | 18888 | Go 性能分析 |
| pprof (pong) | 9999 | Go 性能分析 |
| pprof (sharp_transfer) | 45671 | Go 性能分析 |

---

## 3. 目录结构与文件描述

### 3.1 目录树概览

```
sharpshooter/
├── .gitignore
├── LICENSE
├── Readme.md
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

### 3.2 根目录

| 文件 | 说明 |
|------|------|
| `go.mod` | Go Modules 定义，模块路径 `github.com/soyum2222/sharpshooter`，依赖 `reedsolomon` |
| `go.sum` | 依赖校验和 |
| `.gitignore` | 忽略 `.idea` 目录和 `.exe` 文件 |
| `LICENSE` | MIT 许可证，Copyright 2021 soyum2222 |
| `Readme.md` | 项目说明文档，包含协议格式说明和使用示例 |

### 3.3 核心源文件（根目录）

#### 3.3.1 `sniper.go`

核心文件，定义了 `Sniper` 结构体及其所有方法，实现 `net.Conn` 接口。

**Sniper 结构体主要字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `packageSize` | `int64` | 每个数据包最大载荷大小，默认 800 字节 |
| `rtt` / `rto` | `int64` | 往返时间和重传超时 |
| `winSize` | `int32` | 发送窗口大小，默认 64 |
| `sendId` | `uint32` | 发送序列号计数器 |
| `rcvId` | `uint32` | 接收期望的下一个序列号 |
| `ammoBag` | `[]*protocol.Ammo` | 发送窗口（待确认的数据包） |
| `rcvAmmoBag` | `[]*protocol.Ammo` | 接收窗口（按序排列） |
| `rcvBuffer` | `[][]byte` | 已排序的接收数据缓冲 |
| `sendBuffer` | `[]byte` | 未满一包的发送暂存 |
| `fec` | `bool` | 是否开启 FEC |
| `ackCache` / `ackSendCache` | `[]uint32` | ACK 缓冲与发送缓冲 |

**Statistics 结构体：** 流量统计，包含 TotalTraffic、EffectiveTraffic、TotalPacket、EffectivePacket、RTT、RTO、SendWin、ReceiveWin。

**常量定义：**

| 常量 | 值 | 说明 |
|------|----|------|
| `DEFAULT_HEAD_SIZE` | 20 | 协议头大小（SIZE + SQE + CMD + PROOF） |
| `DEFAULT_INIT_SENDWIND` | 64 | 初始发送窗口大小 |
| `DEFAULT_INIT_RECEWIND` | 1024 | 初始接收窗口大小 |
| `DEFAULT_INIT_PACKSIZE` | 800 | 默认包载荷大小 |
| `DEFAULT_INIT_HEALTHTICKER` | 1 | 健康检查间隔（秒） |
| `DEFAULT_INIT_HEALTHCHECK_TIMEOUT_TRY_COUNT` | 10 | 健康检查最大重试次数 |
| `DEFAULT_INIT_HANDSHACK_TIMEOUT` | 6 | 握手超时重试次数 |
| `DEFAULT_INIT_RTO_UNIT` | 200ms | 初始 RTO 基准 |
| `DEFAULT_INIT_DELAY_ACK` | 200ms | 延迟 ACK 时间 |
| `DEFAULT_INIT_INTERVAL` | 500 | 发送间隔 |

**连接状态枚举（STATUS_*）：**
- `STATUS_NONE` (0) — 初始状态
- `STATUS_SECONDHANDSHACK` (1) — 已发送第二次握手
- `STATUS_THIRDHANDSHACK` (2) — 已发送第三次握手
- `STATUS_NORMAL` (3) — 正常通信
- `STATUS_CLOSEING1-3` (4-6) — 关闭中

**主要方法：**

| 方法 | 说明 |
|------|------|
| `NewSniper(conn, aim)` | 创建新的 Sniper 实例，初始化所有参数 |
| `Write(b []byte)` | 发送数据（实现 `net.Conn`），支持 deadline |
| `Read(b []byte)` | 接收数据（实现 `net.Conn`），支持 deadline 和超时 |
| `Close()` | 关闭连接，发送 CLOSE 包，等待对端确认 |
| `LocalAddr()` / `RemoteAddr()` | 返回本地/远程地址 |
| `SetDeadline(t)` / `SetReadDeadline(t)` / `SetWriteDeadline(t)` | 设置超时 |
| `SetPackageSize(size)` | 设置数据包大小 |
| `SetRecWin(size)` / `SetSendWin(size)` | 设置接收/发送窗口大小 |
| `SetInterval(interval)` | 设置发送间隔 |
| `OpenFec(dataShards, parShards)` | 开启 FEC 模式 |
| `OpenStaTraffic()` | 开启流量统计 |
| `TrafficStatistics()` | 获取流量统计快照 |
| `CleanStatistics()` | 重置统计数据 |
| `Debug()` | 开启调试日志 |
| `shoot(put bool)` | 发送引擎，遍历 ammoBag 中窗口内的包进行发送 |
| `fire(ammo)` | 将单个包经 UDP 发出，记录 RTT 采样时间 |
| `flush()` | 清除 ammoBag 中已被 ACK 确认的包 |
| `autoShoot()` | 定时重传触发器，注册到 TimedSched |
| `ack(id)` | 将收到的包 ID 加入 ACK 缓冲 |
| `ackTimer()` | 定时发送 ACK，周期为 min(rtt/4, 30ms) |
| `ackSender()` | 将 ackSendCache 中的 ACK 批量发送 |
| `wrapACK()` / `unWrapACK()` | ACK 压缩/解压（连续 ACK 用三元组表示） |
| `handleAck(ids)` | 处理收到的 ACK，移除已确认包，触发 RTT 计算 |
| `expandWin()` / `zoomoutWin()` | 扩大/缩小发送窗口 |
| `healthMonitor()` | 健康检查，超时后关闭连接 |
| `copyRcvBuffer(b)` | 将接收缓冲区的数据拷贝到用户提供的 buffer |
| `monitor()` | 独立读取协程（无 headquarters 时使用） |

**发送策略：** `delaySend`（默认）按包分片逐个发送；`fecSend` 将数据分块后进行 FEC 编码发送。

#### 3.3.2 `dispatch.go`

服务端核心路由与连接管理。

**headquarters 结构体：** UDP 服务器端管理器。

| 字段 | 说明 |
|------|------|
| `conn` | UDP 连接 |
| `Snipers` | `sync.Map`，按地址存储所有活跃的 Sniper 连接 |
| `accept` | 新连接通道，Accept() 从此取连接 |
| `blockSign` | 读阻塞信号 |
| `errorSign` | 错误信号 |
| `closeSign` | 关闭信号 |

**主要函数：**

| 函数 | 说明 |
|------|------|
| `Dial(addr)` | 客户端主动连接，执行三次握手，返回 `net.Conn` |
| `Listen(addr)` | 服务端监听，返回 `*headquarters` |
| `NewHeadquarters()` | 创建 headquarters 实例 |
| `Accept()` | 接受新连接（返回 `net.Conn`） |
| `Close()` | 关闭服务器 |
| `Addr()` | 返回监听地址 |
| `monitor()` | 主读取循环，接收 UDP 包并路由分发 |
| `routing(sn, msg)` | 消息路由，根据 CMD 类型分发到对应处理函数 |
| `clear()` | 定期清理已关闭的 Sniper 连接 |
| `ReadFrom(b)` | 从所有连接中读取数据（用于无连接编号的场景） |
| `WriteToAddr(b, addr)` | 向指定地址的连接写入数据 |

#### 3.3.3 `handshack.go`

三次握手实现。

| 函数 | 说明 |
|------|------|
| `firstHandShack(h, remote)` | 处理第一次握手（服务端），创建新 Sniper 并回复第二次握手 |
| `secondHandShack(h, remote, id)` | 处理第二次握手（客户端发起连接时），回复第三次握手 |
| `thirdHandShack(h, remote, id)` | 处理第三次握手（服务端），验证 ID 后将连接放入 accept 通道 |

握手流程：
```
Client                          Server
  |---- FIRSTHANDSHACK ---------->|
  |<--- SECONDHANDSHACK ----------|  (携带随机 ID)
  |---- THIRDHANDSHACK ---------->|  (回传 ID 验证)
  |          连接建立               |
```

#### 3.3.4 `receive.go`

数据包接收与排序。

| 函数 | 说明 |
|------|------|
| `rcvnoml(ammo)` | 普通模式接收：按 sequence 排序，连续的包合并到 rcvBuffer |
| `rcvfec(ammo)` | FEC 模式接收：收集一整组分片后解码，恢复原始数据 |

两种模式都通过 `int(ammo.Id) - int(s.rcvId)` 计算 bagIndex 来处理序列号回绕问题。

#### 3.3.5 `wrap.go`

数据封装与发送封装。

| 函数 | 说明 |
|------|------|
| `wrapnoml()` | 普通模式：将 sendBuffer 按 packageSize 分片为 Ammo 包发送 |
| `wrapfec()` | FEC 模式：将 sendBuffer 按 `dataShards * packageSize` 分块，编码为 data + parity 分片发送 |

#### 3.3.6 `read.go`

`Sniper.Read()` 实现，阻塞等待数据到达。支持 deadline 和超时机制。使用 `readBlock` channel 通知新数据到达，`closeChan` 通知连接关闭。

#### 3.3.7 `fec.go`

FEC（前向纠错）编解码器，基于 Reed-Solomon 算法。

| 类型 | 说明 |
|------|------|
| `fecEncoder` | 编码器，包含 dataShards、parShards 和 reedsolomon.Encoder |
| `fecDecoder` | 解码器，包含 dataShards、parShards 和 reedsolomon.Encoder |

| 方法 | 说明 |
|------|------|
| `newFecEncoder(data, par)` | 创建编码器 |
| `newFecDecoder(data, par)` | 创建解码器 |
| `encode(b)` | 编码：添加 4 字节长度前缀后分片 + 编码冗余片 |
| `decode(b)` | 解码：Reconstruct 恢复缺失片，Join 合并原始数据 |
| `estimatedLength(length)` | 估算 FEC 编码后的总长度 |

#### 3.3.8 `rtt.go`

RTT/RTO 计算。

`calrto(rtt)` — 根据最新 RTT 采样值更新 SRTT（平滑 RTT）和 RTO（重传超时）：
- 首次采样：`SRTT = rtt`, `RTO = rtt * 2`
- 后续采样：`SRTT = 0.3 * rtt + 0.7 * SRTT`, `RTO = SRTT * 1.5`
- 忽略异常采样（rtt > 2 * current_rtt）

#### 3.3.9 `closechan.go`

安全的 channel 关闭工具，防止重复关闭 panic。

`chanCloser` 结构体使用互斥锁保护 `close` 操作，通过 select 检查 channel 是否已关闭。

#### 3.3.10 `headquarters.go`

定时调度系统，参考 kcp-go 实现。

`TimedSched` — 基于 heap 的全局定时任务调度器，使用 CPU 核数个 worker 并行执行。

| 方法 | 说明 |
|------|------|
| `NewTimedSched(parallel)` | 创建调度器，启动 parallel 个 worker goroutine |
| `Put(f, deadline)` | 提交定时任务 |
| `Close()` | 关闭调度器 |

`SystemTimedSched` — 全局单例，在库加载时创建。

#### 3.3.11 `sharpshooter_test.go`

核心测试文件，覆盖以下测试场景：

| 测试函数 | 说明 |
|----------|------|
| `TestCopyRcvBuffer` | 测试接收缓冲区拷贝逻辑 |
| `TestDial` | 测试 Dial/Accept 建立连接 |
| `TestSniper_Close` | 测试连接关闭（两端） |
| `TestSniper_Close2` | 测试发送后关闭连接 |
| `TestSniper_ClientClose` | 测试客户端主动关闭 |
| `TestSniper_ServerClose` | 测试服务端主动关闭 |
| `TestSniper_WriteCloseConn` | 测试关闭后再写入返回错误 |
| `TestSniper_ReadCloseConn` | 测试关闭后再读取返回错误 |
| `TestSniper_SetDeadline` | 测试通用 deadline |
| `TestSniper_SetReadDeadline` | 测试读 deadline |
| `TestSniper_SetWriteDeadline` | 测试写 deadline |
| `Test_CreateLargeConnection` | 测试 1024 个并发连接 |
| `TestUnWrapACK` | 测试 ACK 解压缩 |
| `TestSendBigData` | 测试发送 1024 个 1MB 数据块 |

所有连接测试都开启 FEC (4,3)。

#### 3.3.12 `fec_test.go`

FEC 编解码测试 — `TestFecEncode`：编码 16 字节数据为 6 片 (4+2)，丢弃 2 片后解码恢复。

### 3.4 protocol/ 包

#### 3.4.1 `protocol.go`

协议层实现，定义数据包格式和序列化。

**Ammo 结构体（数据包）：**

| 字段 | 类型 | 大小 | 说明 |
|------|------|------|------|
| `Length` | `uint32` | 4 bytes | Body + 10 的总长度（不含 Length 字段自身） |
| `Id` | `uint32` | 4 bytes | 序列号 |
| `Kind` | `uint16` | 2 bytes | 包类型（CMD） |
| `proof` | `uint32` | 4 bytes | 校验值（Body 中所有 bit 为 1 的数量） |
| `Body` | `[]byte` | 变长 | 载荷数据 |

**包类型枚举（CMD）：**

| 值 | 常量 | 说明 |
|----|------|------|
| 0 | `ACK` | 确认包 |
| 1 | `NORMAL` | 普通数据包 |
| 2 | `FIRSTHANDSHACK` | 第一次握手 |
| 3 | `SECONDHANDSHACK` | 第二次握手 |
| 4 | `THIRDHANDSHACK` | 第三次握手 |
| 5 | `CLOSE` | 关闭连接 (FIN) |
| 6 | `CLOSERESP` | 关闭连接响应 |
| 7 | `HEALTHCHECK` | 健康检查 |
| 8 | `HEALTCHRESP` | 健康检查响应 |
| 9 | `NORMALTAIL` | 带关闭标记的最后一个数据包 |
| 10 | `OUTOFAMMO` | 保留 |

**校验机制：** `proof` 字段存储 Body 中所有字节 bit 为 1 的计数之和（使用预计算查找表 `table`）。`Marshal` 时计算并写入，`Unmarshal` 时验证。

**主要函数：**

| 函数 | 说明 |
|------|------|
| `Marshal(ammo)` | 将 Ammo 序列化为字节流 |
| `Unmarshal(b)` | 从字节流反序列化为 Ammo，校验长度和 proof |
| `Free()` | 重置 Ammo 所有字段，归还对象池 |

**Ammo 辅助方法：**
- `AckAdd()` — ACK 计数 +1
- `ShootAdd()` / `ShootCount()` — 发送计数递增/读取

#### 3.4.2 `protocol_test.go`

协议测试：

| 测试函数 | 说明 |
|----------|------|
| `TestMarshalUnmarshal` | 序列化/反序列化往返测试 |
| `TestRogue` | 恶意包检测（错误长度） |
| `BenchmarkMarshal` | 序列化基准测试 |
| `BenchmarkUnmarshal` | 反序列化基准测试 |

### 3.5 tool/ 包

#### 3.5.1 `tool/time_consuming.go`

性能计时工具。

`TimeConsuming()` — 返回一个 `defer` 函数，用于测量函数执行时间。调用 `runtime.Caller(1)` 获取调用者函数名并打印耗时（纳秒）。

用法：
```go
defer tool.TimeConsuming()()
```

#### 3.5.2 `tool/block/block.go`

协程阻塞/唤醒原语。

**Blocker 结构体：**

| 方法 | 说明 |
|------|------|
| `NewBlocker()` | 创建新 Blocker |
| `Block()` | 阻塞当前协程，直到 `Pass()` 或 `Close()` |
| `Pass()` | 唤醒一个阻塞的协程 |
| `PassBT(duration)` | 在指定时间内尝试唤醒，超时返回 |
| `Select()` | 返回一个 channel，可嵌入 select |
| `Close()` | 唤醒所有阻塞协程并永久关闭 Blocker |

在 Sniper 中用于 Write 流控：当发送窗口满时 `Block()` 阻塞，窗口有空位时 `Pass()` 唤醒。

#### 3.5.3 `tool/block/block_test.go`

Blocker 测试：单次阻塞/唤醒、多协程阻塞/唤醒、顺序性验证、Close 唤醒所有。

### 3.6 example/ 目录

| 文件 | 说明 |
|------|------|
| `ping.go` | 客户端示例，连接后开启 FEC(10,3)，循环发送 "ping" 并接收 "pong"，带 pprof 监听 |
| `pong.go` | 服务端示例，监听后接受连接开启 FEC(10,3)，接收 "ping" 回复 "pong"，带 pprof 监听 |
| `server_send.go` | 文件发送服务端，监听后将 `./test` 文件内容通过 `io.Copy` 发送 |
| `client_receive.go` | 文件接收客户端，连接后循环读取数据（丢弃） |
| `sharp_transfer.go` | 完整文件传输工具，支持命令行参数、断点续传、TCP/Sharpshooter 对比、进度条、速度显示、调试模式 |

### 3.7 image/ 目录

| 文件 | 说明 |
|------|------|
| `network.png` | 传输速度截图 |
| `network-utilization.png` | 网络利用率截图 |

---

## 4. 协议格式

### 4.1 数据包格式

```
| SIZE(4byte) | SQE(4byte) | CMD(2byte) | PROOF(4byte) | CONTENT(.......) |
```

| 字段 | 大小 | 说明 |
|------|------|------|
| SIZE | 4 bytes | 包含 SQE + CMD + PROOF + CONTENT 的总字节数（不含自身） |
| SQE | 4 bytes | 序列号，连续数据包的 SQE 连续递增 |
| CMD | 2 bytes | 包类型 |
| PROOF | 4 bytes | 校验和，Body 中所有 bit 为 1 的数量 |
| CONTENT | 变长 | 载荷 |

数据包最大长度不能超过 `DEFAULT_INIT_PACKSIZE`（800）或用户自定义的 `packageSize`。

### 4.2 ACK 包格式

```
| SIZE(4byte) | SQE(4byte) | CMD(2byte) | ackSQE1(4byte)| ackSQE2(4byte) | ackSQE3(4byte) | ... |
```

**ACK 压缩规则：**
- 连续 ACK 少于 3 个：逐个列出
- 连续 ACK 大于等于 3 个：用三元组 `(start, start, end)` 表示连续区间
  - 例：`|5|5|10|` 表示 ACK 5 到 10

### 4.3 连接状态机

```
STATUS_NONE ─────────────────────┐
    │ firstHandShack (server)     │ secondHandShack (client-side)
    ▼                             ▼
STATUS_SECONDHANDSHACK    STATUS_THIRDHANDSHACK
    │ thirdHandShack (server)     │ (transient)
    ▼                             
STATUS_NORMAL ──── 数据传输 ────► STATUS_NORMAL
    │
    │ Close()
    ▼
STATUS_CLOSEING → 连接关闭
```

---

## 5. 关键业务逻辑

### 5.1 连接建立（三次握手）

1. **Client → Server:** `FIRSTHANDSHACK` — 客户端发起连接请求
2. **Server → Client:** `SECONDHANDSHACK` — 服务端创建 Sniper，生成随机 ID，回复
3. **Client → Server:** `THIRDHANDSHACK` — 客户端回传 ID，服务端验证
4. 连接建立，开始数据传输

握手过程中每步都有状态验证，防止恶意/重复包。

### 5.2 数据发送与重传

**发送流程：**
1. 应用调用 `Write(data)`
2. 数据暂存到 `sendBuffer`，按 `packageSize` 分片为 Ammo
3. Ammo 放入发送窗口 `ammoBag`，通过 `fire()` 发送
4. `autoShoot()` 定时器周期性触发重传（间隔 = min(RTO, interval)）
5. 收到 ACK 后 `handleAck()` 移除已确认的包
6. `flush()` 压缩 ammoBag 移除头部 nil 空洞
7. 窗口有空间时 `writerBlocker.Pass()` 唤醒等待的 Write

**ACK 延迟发送：**
- ACK 缓存在 `ackCache` 中，由 `ackTimer()` 定时批量发送
- 发送周期为 `min(rtt/4, 30ms)`
- 当 ACK 缓存超过 `packageSize/4` 时立即发送
- 连续 ACK 使用三元组压缩减少带宽

### 5.3 滑动窗口拥塞控制

- **窗口扩大：** 当一周期内发送量与窗口比 > 0.9 且丢包率低时，`winSize *= 1.25`，上限 32768
- **窗口缩小：** 当丢包率 > 50% 时，`winSize /= 1.25`，下限为 `minSize`（默认 64）

### 5.4 RTT/RTO 计算

- 每个 cycle 对第一个发出的包进行 RTT 采样
- 首次采样：`SRTT = rtt`, `RTO = rtt * 2`
- 后续：`SRTT = 0.3 * newRTT + 0.7 * SRTT`, `RTO = SRTT * 1.5`
- 异常值过滤：忽略 RTT > 2 * current_SRTT 的采样

### 5.5 FEC 前向纠错

使用 Reed-Solomon 纠删码：
- 数据被分为 `dataShards` 个分片，额外生成 `parShards` 个冗余片
- 接收端只要收到 `dataShards` 个片即可恢复原始数据
- 最多容忍 `parShards` 个片丢失
- 默认示例参数：dataShards=10, parShards=3（容忍 30% 丢包）

### 5.6 健康检查

- 每 1 秒发送 `HEALTHCHECK` 包
- 对端收到后回复 `HEALTCHRESP`
- 连续 10 次未收到回复则判定连接断开
- 断开后关闭所有相关 channel 和连接

### 5.7 连接关闭

1. 主动方调用 `Close()`
2. 等待 ammoBag 中所有包发送完毕
3. 发送 `CLOSE` 包
4. 被动方收到后将最后一个数据包标记为 `NORMALTAIL`
5. 被动方发送完剩余数据后回复 `CLOSERESP`
6. 双方关闭连接

---

## 6. 注意事项

### 6.1 开发规范

- 实现了 Go 标准库 `net.Conn` 接口，可直接替换 TCP 使用
- 使用 `sync.Pool` 复用 Ammo 对象，减少 GC 压力
- 全局定时调度器 `SystemTimedSched` 使用 CPU 核数并行

### 6.2 已知限制

- Go 版本要求 1.15+（较旧）
- FEC 编解码有一定 CPU 开销
- 序列号使用 `uint32`，极长时间连接可能回绕（代码中通过 int32 差值处理）
- `rand.Seed` 在 `handshack.go` 的 `init()` 中调用，Go 1.20+ 已弃用
- `Readme.md` 中的文件传输示例链接有误（两个链接指向同一文件）

### 6.3 安全注意事项

- 协议校验使用简单的 bit 计数，不是密码学安全的校验
- 无加密机制，数据明文传输
- 无认证机制，适合在可信网络或 VPN 上使用
- 适合用于绕过协议特征检测（非内容检测）

### 6.4 运维注意事项

- 示例程序内置了 `net/http/pprof` 支持，可通过 HTTP 端口进行性能分析
- 网络利用率取决于发送窗口大小和网络状况
- 可通过 `OpenStaTraffic()` 开启流量统计来监控传输效率

---

## 7. 依赖清单

### 7.1 直接依赖

| 依赖 | 版本 | 用途 |
|------|------|------|
| `github.com/klauspost/reedsolomon` | v1.9.9 | Reed-Solomon 纠删码，用于 FEC 前向纠错 |

### 7.2 标准库使用

| 包 | 用途 |
|----|------|
| `net` | UDP 网络通信 |
| `sync` / `sync/atomic` | 并发控制 |
| `container/heap` | 定时调度器堆 |
| `encoding/binary` | 大端字节序编解码 |
| `time` | 定时器、deadline |
| `math` | 数学计算 |
| `sort` | ACK 排序 |
| `runtime` | 获取 CPU 核数 |
| `errors` | 错误定义 |
| `fmt` | 调试输出 |
| `io` | io.ReadFull / io.Copy |
| `os` | 文件操作 |
| `testing` | 单元测试 |
