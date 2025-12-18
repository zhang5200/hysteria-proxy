# Hysteria2 流量统计 API 参考文档

## API 基本信息

- **服务器地址**: `http://66.154.105.113:8080`
- **认证方式**: HTTP Header `Authorization: zx8257686@520`
- **数据格式**: JSON

---

## API 接口详解

### 1. 获取在线用户 - GET /online

查看当前在线的用户及其连接数（设备数）。

**请求示例:**

```bash
curl -H 'Authorization: zx8257686@520' http://66.154.105.113:8080/online
```

**响应格式:**

```json
{
  "用户名1": 连接数,
  "用户名2": 连接数
}
```

**实际响应示例:**

```json
{ "user": 1 }
```

**字段说明:**

- **键（用户名）**: 认证时使用的用户名
- **值（连接数）**: 该用户当前的连接数，表示有多少个 Hysteria 客户端实例（设备）在线
  - `1` = 1 个设备在线
  - `2` = 2 个设备在线（例如手机和电脑同时连接）

**空响应:**

```json
{}
```

表示当前没有用户在线。

---

### 2. 获取流量统计 - GET /traffic

查看所有用户的流量使用情况（累计统计）。

**请求示例:**

```bash
curl -H 'Authorization: zx8257686@520' http://66.154.105.113:8080/traffic
```

**响应格式:**

```json
{
  "用户名1": {
    "tx": 上传字节数,
    "rx": 下载字节数
  },
  "用户名2": {
    "tx": 上传字节数,
    "rx": 下载字节数
  }
}
```

**实际响应示例:**

```json
{
  "user": {
    "tx": 1048576,
    "rx": 5242880
  }
}
```

**字段说明:**

- **tx (transmit)**: 用户上传的字节数（从客户端发送到服务器）
- **rx (receive)**: 用户下载的字节数（从服务器发送到客户端）

**流量单位换算:**

- `1,024 Bytes` = 1 KB
- `1,048,576 Bytes` = 1 MB
- `1,073,741,824 Bytes` = 1 GB

**示例换算:**

```
tx: 1048576 = 1 MB (上传)
rx: 5242880 = 5 MB (下载)
```

**清零统计:**

```bash
# 获取流量数据后立即清零
curl -H 'Authorization: zx8257686@520' http://66.154.105.113:8080/traffic?clear=1
```

---

### 3. 踢出用户 - POST /kick

强制断开指定用户的连接。

**请求示例:**

```bash
curl -X POST \
  -H 'Authorization: zx8257686@520' \
  -H 'Content-Type: application/json' \
  -d '["user"]' \
  http://66.154.105.113:8080/kick
```

**请求体格式:**

```json
["用户名1", "用户名2"]
```

**注意事项:**

- 被踢出的用户会立即断开连接
- 客户端会自动尝试重新连接
- 如需永久封禁，需要同时修改认证配置

---

### 4. 获取 TCP 流详情 - GET /dump/streams

查看当前活跃的 TCP 连接详细信息。

**请求示例:**

```bash
curl -H 'Authorization: zx8257686@520' http://66.154.105.113:8080/dump/streams
```

**响应格式:**

```json
{
  "连接ID": {
    "user": "用户名",
    "remote": "客户端地址",
    "target": "目标地址",
    "tx": 上传字节数,
    "rx": 下载字节数
  }
}
```

---

## 使用场景示例

### 场景 1: 监控在线用户数

```bash
# 每分钟检查一次在线用户
watch -n 60 'curl -s -H "Authorization: zx8257686@520" http://66.154.105.113:8080/online'
```

### 场景 2: 查看流量使用情况

```bash
# 查看当前流量统计
curl -H 'Authorization: zx8257686@520' http://66.154.105.113:8080/traffic | jq

# 输出示例（格式化后）:
# {
#   "user": {
#     "tx": 1048576,    # 上传 1 MB
#     "rx": 5242880     # 下载 5 MB
#   }
# }
```

### 场景 3: 定时收集流量数据

```bash
#!/bin/bash
# 每小时收集一次流量数据并清零

while true; do
  timestamp=$(date '+%Y-%m-%d %H:%M:%S')
  traffic=$(curl -s -H 'Authorization: zx8257686@520' \
    'http://66.154.105.113:8080/traffic?clear=1')

  echo "$timestamp - $traffic" >> traffic_log.txt
  sleep 3600  # 等待 1 小时
done
```

### 场景 4: 流量告警

```bash
#!/bin/bash
# 当流量超过 10GB 时发送告警

traffic=$(curl -s -H 'Authorization: zx8257686@520' \
  http://66.154.105.113:8080/traffic)

# 解析 JSON 并检查流量（需要 jq 工具）
total_rx=$(echo $traffic | jq -r '.user.rx // 0')

# 10GB = 10737418240 字节
if [ $total_rx -gt 10737418240 ]; then
  echo "警告: 流量已超过 10GB!"
  # 这里可以添加发送邮件或通知的代码
fi
```

---

## Python 调用示例

```python
import requests
import json

API_URL = "http://66.154.105.113:8080"
HEADERS = {"Authorization": "zx8257686@520"}

# 获取在线用户
def get_online_users():
    response = requests.get(f"{API_URL}/online", headers=HEADERS)
    return response.json()

# 获取流量统计
def get_traffic_stats():
    response = requests.get(f"{API_URL}/traffic", headers=HEADERS)
    return response.json()

# 踢出用户
def kick_users(usernames):
    response = requests.post(
        f"{API_URL}/kick",
        headers={**HEADERS, "Content-Type": "application/json"},
        data=json.dumps(usernames)
    )
    return response.status_code

# 使用示例
if __name__ == "__main__":
    # 查看在线用户
    online = get_online_users()
    print(f"在线用户: {online}")
    # 输出: 在线用户: {'user': 1}

    # 查看流量统计
    traffic = get_traffic_stats()
    for user, stats in traffic.items():
        tx_mb = stats['tx'] / 1024 / 1024
        rx_mb = stats['rx'] / 1024 / 1024
        print(f"用户 {user}: 上传 {tx_mb:.2f} MB, 下载 {rx_mb:.2f} MB")
```

---

## 常见问题

### Q1: 为什么 /online 返回的是连接数而不是流量？

**A:** `/online` 接口专门用于查看在线状态，返回的数字表示该用户有多少个设备（客户端实例）在线。流量信息需要调用 `/traffic` 接口获取。

### Q2: tx 和 rx 是从谁的角度统计的？

**A:** 从**客户端**的角度统计：

- `tx` = 客户端上传（发送）的数据
- `rx` = 客户端下载（接收）的数据

### Q3: 流量统计会自动清零吗？

**A:** 不会自动清零。流量会一直累计，除非：

- 手动调用 `/traffic?clear=1` 清零
- 重启 Hysteria2 服务

### Q4: 如何将字节转换为 GB？

**A:**

```
GB = 字节数 / 1024 / 1024 / 1024
```

或者

```
GB = 字节数 / 1073741824
```

---

## 安全提示

⚠️ **重要安全建议:**

1. **保护 API 密钥**: 不要在公开的代码仓库中暴露 `Authorization` 密钥
2. **限制访问 IP**: 建议配置防火墙，只允许特定 IP 访问 8080 端口
3. **使用 HTTPS**: 生产环境建议通过 Nginx 等反向代理添加 SSL/TLS 加密
4. **定期更换密钥**: 定期修改 `config.yaml` 中的 `secret` 值

---

## 更新日志

- **2025-12-17**: 初始版本，添加所有 API 接口说明
- 服务器地址: `66.154.105.113:8080`
- 当前在线用户: `user` (1 个连接)
