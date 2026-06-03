<h1 align="center">Sharpshooter</h1>

<p align="center">
  <b>基于 UDP 的可靠传输协议（Go 实现）</b>
</p>

<p align="center">
  <img src="https://img.shields.io/github/license/soyum2222/sharpshooter?logo=Github&style=flat-square" />
  <img src="https://img.shields.io/github/go-mod/go-version/soyum2222/sharpshooter?logo=Go&style=flat-square" />
  <img src="https://img.shields.io/github/v/tag/soyum2222/sharpshooter?label=version&style=flat-square" />
  <img src="https://img.shields.io/github/commit-activity/m/soyum2222/sharpshooter?logo=Github&style=flat-square" />
</p>

<p align="center">
  <a href="Readme.md">English</a>
</p>

---

Sharpshooter 是一个基于 UDP 的可靠传输协议库，使用 Go 语言实现。它提供类似 TCP 的面向连接语义，但没有 TCP 的协议特征，可用于绕过协议特征检测，也可作为 P2P 应用的传输层。

**特性：**

- 类 TCP 三次握手
- ACK 确认与重传机制
- 自适应滑动窗口拥塞控制
- 可选的 FEC 前向纠错（基于 Reed-Solomon 纠删码）
- RTT/RTO 自动校准
- 健康检查与超时断连
- 实现了 `net.Conn` 接口，可直接替换 TCP 使用
- 除纠删码外无其他外部依赖

---

## 快速开始

```bash
go get github.com/soyum2222/sharpshooter
```

### 服务端

```go
l, _ := sharpshooter.Listen(":8858")
conn, _ := l.Accept()
// conn 实现了 net.Conn，直接使用 Read/Write 即可
```

### 客户端

```go
conn, _ := sharpshooter.Dial("127.0.0.1:8858")
// 开启 FEC（可选），可容忍 30% 丢包
conn.(*sharpshooter.Sniper).OpenFec(10, 3)
conn.Write([]byte("hello"))
```

更多示例见 [`example/`](https://github.com/soyum2222/sharpshooter/tree/master/example) 目录。

---

## API

| 方法 | 说明 |
|------|------|
| `Dial(addr) (net.Conn, error)` | 连接远端监听器 |
| `Listen(addr) (*headquarters, error)` | 启动 UDP 监听 |
| `Accept() (net.Conn, error)` | 接受新连接 |
| `OpenFec(data, par)` | 开启 FEC（如 `OpenFec(10, 3)` 可容忍 30% 丢包） |
| `SetPackageSize(size)` | 设置包载荷大小 |
| `SetSendWin(size)` / `SetRecWin(size)` | 设置发送/接收窗口大小 |
| `OpenStaTraffic()` | 开启流量统计 |
| `TrafficStatistics()` | 获取流量统计快照 |

支持 `net.Conn` 的所有标准方法（`Read`、`Write`、`Close`、`SetDeadline` 等）。

协议格式与详细文档见 [`docs/`](docs/)。

---

## 文件传输工具

```bash
# 接收端
go run example/sharp_transfer.go -l 8858 -o output.dat

# 发送端
go run example/sharp_transfer.go -addr 127.0.0.1:8858 -i input.dat

# 断点续传
go run example/sharp_transfer.go -addr 127.0.0.1:8858 -i input.dat -o output.dat -c

# 与 TCP 对比
go run example/sharp_transfer.go -addr 127.0.0.1:8858 -i input.dat -t
```

---

## 相关项目

- [sharpshooter-tunel](https://github.com/soyum2222/sharpshooter-tunel) — TCP 与 Sharpshooter 互转工具

## 许可证

[MIT](LICENSE)
