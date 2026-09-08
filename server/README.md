# Go 服务开发规范

## 目录职责

```text
cmd/
  api/                    服务入口：参数、配置、日志和进程信号
  conntest/               使用相同配置检查依赖连接
api/
  router/                 URL 注册、中间件绑定
  handler/                HTTP 控制器、鉴权入口、WebSocket 升级和静态文件
  request/                请求 DTO、严格 JSON 解析
  response/               code/message/data/requestId 统一响应
middleware/               访问日志、请求 ID、超时、panic 恢复和全局错误处理
config/                   YAML 读取、环境变量覆盖与启动校验
config.yml                当前本地配置，已忽略提交
config.example.yml        带注释的配置模板
model/
  entity/                 用户、对局结算和战绩实体
pkg/
  auth/                   JWT、密码哈希和用户名校验
  database/               MySQL 连接池与连通性检查
  errors/                 统一错误码、公开消息与内部原因
  logger/                 JSON 日志、脱敏、轮转和后台 panic 日志
  requestctx/             请求追踪标识
internal/
  app/                    MySQL / Redis / HTTP / WebSocket 装配与退出
  service/                账户业务流程及仓储接口
  repository/mysql/       建表、用户查询和对局事务
  room/                   房间业务，按职责拆分文件
    hub.go                本地连接索引、后台任务、关闭
    client.go             WebSocket 读写、心跳、背压和连接清理
    protocol.go           入站和出站事件
    handlers.go           消息路由、创建与加入房间
    lifecycle.go          入座、离开与断线处理
    moves.go              玩家行动、AI、认输与再来一局
    match.go              Redis 匹配队列
    broadcast.go          跨实例订阅与广播
    store.go              Redis 房间状态与分布式锁
    settlement.go         对局结算与失败日志
    errors.go             游戏领域错误到公开错误码的映射
  game/                   纯规则引擎
  ai/                     AI 搜索
  web/                    构建时嵌入前端
```

目录分层参考同级 `gin` 项目。HTTP 调用顺序：`api/router -> middleware -> api/handler -> internal/service -> internal/repository/mysql`；请求 DTO 放 `api/request`，实体放 `model/entity`，统一响应放 `api/response`，数据库连接等基础能力放 `pkg`。

HTTP 服务使用标准库 `net/http`，接口地址和 JSON / WebSocket 协议保持兼容。房间是独立的实时业务模块，内部管理 Redis 状态和连接；`internal/game` 和 `internal/ai` 保留纯游戏逻辑。配置直接编辑服务根目录的 `config.yml`。

## 启动与构建

在项目根目录运行 `./build.sh`，构建前端并将资源同步到 `internal/web/dist`，再生成 `server/penguin-chess-server`。

```bash
cd server
cp config.example.yml config.yml
# 配置 MySQL、Redis 和独立 JWT 密钥
chmod 600 config.yml
./penguin-chess-server
# 可覆盖端口和配置文件路径
./penguin-chess-server -addr 127.0.0.1:8081 -config config.yml
```

已有的本地配置已迁移到 `config.yml`，直接编辑该文件即可，不要用示例覆盖自己的配置。配置按 `server`、`mysql`、`redis`、`auth`、`log` 分组，各字段含注释；修改后重启服务生效。旧 `.env.local` 已备份为 `.env.local.bak`，服务不再读取它；本次格式迁移保留已有 JWT 密钥，不会使会话失效。

优先级为命令行 `-addr` > 环境变量 > YAML > 默认值。默认加载启动目录的 `config.yml`，可通过 `-config /path/to/config.yml` 指定文件；生产环境仅注入环境变量时使用 `-config ''`。指定文件不存在、未知字段、重复字段、多个 YAML 文档或非法类型均报错，避免配置拼写错误被静默忽略。相对路径按进程工作目录解析。

MySQL 数据库需预先创建，启动时幂等创建现有两张表，不自动修改已有列。`go run ./cmd/conntest -config config.yml` 仅验证连接，不创建数据库、不修改业务数据。

### YAML 字段与环境变量

| YAML 字段 | 环境变量 |
| --- | --- |
| `server.addr` | `PENGUIN_ADDR` |
| `server.instance_id` | `PENGUIN_INSTANCE_ID` |
| `server.request_timeout` | `PENGUIN_REQUEST_TIMEOUT` |
| `server.startup_timeout` | `PENGUIN_STARTUP_TIMEOUT` |
| `server.shutdown_timeout` | `PENGUIN_SHUTDOWN_TIMEOUT` |
| `mysql.dsn` | `PENGUIN_MYSQL_DSN` |
| `redis.addr` | `PENGUIN_REDIS_ADDR` |
| `redis.password` | `PENGUIN_REDIS_PASS` |
| `auth.jwt_secret` | `PENGUIN_JWT_SECRET` |
| `log.level` | `PENGUIN_LOG_LEVEL` |
| `log.file` | `PENGUIN_LOG_FILE` |
| `log.max_size_mb` | `PENGUIN_LOG_MAX_SIZE_MB` |
| `log.max_backups` | `PENGUIN_LOG_MAX_BACKUPS` |
| `log.max_age_days` | `PENGUIN_LOG_MAX_AGE_DAYS` |

启动检查有期限，失败返回非零退出码。收到 SIGINT / SIGTERM 后停止接收 HTTP 请求，等待在途请求，关闭 WebSocket 和订阅任务，再释放 Redis、MySQL 和日志文件。

## Docker Compose

项目根目录的 `docker-compose.yml` 只运行企鹅棋服务；MySQL 和 Redis 使用外部地址 `100.66.1.4:3306`、`100.66.1.4:6379`，不会创建数据库容器。Dockerfile 分阶段构建前端及 Go 程序，最终镜像内嵌前端资源。

当前已生成 `server/config.docker.yml`，沿用本地账号和 JWT 密钥，并将数据库地址改为上述 IP。它与本地预览的 `server/config.yml` 独立。部署前确认目标数据库存在、账号可用，且 Docker 宿主机和容器网络能访问 `100.66.1.4`。

在项目根目录运行：

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f --tail=100 penguins
```

默认访问 `http://宿主机IP:8081`。端口冲突时使用 `PENGUIN_PORT=8083 docker compose up -d --build`；它只改变对外端口，容器内仍监听 8080。修改 Docker YAML 配置后执行 `docker compose restart penguins`。

- 配置只读挂载：`server/config.docker.yml -> /app/config.yml`。
- 日志持久化：`server/logs/docker -> /app/logs`，应用和 Docker 控制台日志分别限制大小并轮转。
- 健康检查：每 30 秒请求 `/readyz`，检查应用及 MySQL、Redis 连接。
- 退出：SIGTERM，最多等待 20 秒；调整应用关闭期限时同步增加 `stop_grace_period`。

源码迁移到新机器时，需一并安全传输 `server/config.docker.yml`；如果该文件不存在，可参考 `server/config.docker.example.yml` 创建并填写账号、密码和至少 32 字节 JWT 密钥。真实配置、日志、宿主机依赖和本地二进制均由 `.dockerignore` 排除，不会进入镜像构建上下文。

## 统一 HTTP 返回

所有 `/api/*`、`/healthz`、`/readyz` 返回相同结构。HTTP 状态码表达传输结果，`code` 表达稳定业务结果；客户端不能通过中文消息判断错误类别。

```json
{
  "code": 0,
  "message": "成功",
  "data": {"players": []},
  "requestId": "c5ad1735-12c2-4f80-a71d-760be372d377"
}
```

```json
{
  "code": 20001,
  "message": "请先登录或重新登录",
  "data": null,
  "requestId": "c5ad1735-12c2-4f80-a71d-760be372d377"
}
```

`X-Request-ID` 与响应体一致，由服务端生成。未知错误和 panic 均使用 `50000`，内部地址、SQL、堆栈不出现在客户端响应中。未知 API 为 JSON 404，错误方法为 JSON 405 并带 `Allow`，不会回退为前端页面。

JSON 请求上限 16 KiB，拒绝未知字段、畸形 JSON 和第二个 JSON 文档。账户接口查询使用请求 context。请求 context 超时要求底层操作配合取消，不能强制停止不响应 context 的 CPU 代码；HTTP 读超时另外限制慢速请求体。

| 接口 | 方法 | 鉴权 |
| --- | --- | --- |
| `/api/register` | POST | 无 |
| `/api/login` | POST | 无 |
| `/api/profile` | GET | `Authorization: Bearer ...` |
| `/api/leaderboard` | GET | 无 |
| `/healthz` | GET | 无，进程存活 |
| `/readyz` | GET | 无，2 秒内检查 MySQL / Redis |
| `/ws` | GET | 无 token 为游客；无效 token 返回 401 |

JWT 使用独立的至少 32 字节密钥。更换密钥会使已有令牌失效，用户需重新登录。本次迁移已将原来的公共默认密钥替换为随机本地密钥。

## 错误码规范

定义集中在 `pkg/errors/error.go`。公开错误值不可修改，通过 `WithCause` 包装内部原因；使用 `errors.Is` / `errors.As` 判断，不匹配字符串。

| 范围 | 含义 |
| --- | --- |
| 0 | 成功 |
| 10001–10004 | 参数、资源不存在、方法不支持、请求过大 |
| 20001–20005 | 未登录、密码错误、无权限、用户名冲突、用户不存在 |
| 30001–30004 | 房间关闭、满员、已在房间、不在房间 |
| 30005–30009 | 回合错误、非法走子、已结束、阶段错误、操作过频 |
| 50000 | 未预期内部错误 |
| 50001 | 依赖暂时不可用 |

WebSocket 成功事件保留 `type/state/last/...` 协议，避免破坏对局同步。错误事件统一使用同一套错误码，`msg` 仅为旧客户端兼容字段：

```json
{"type":"error","code":30005,"message":"还没轮到你","msg":"还没轮到你","data":null,"requestId":"..."}
```

WebSocket 错误的 `requestId` 对应升级请求，可结合 `connection_id` 和 `room_id` 查找该连接的后续记录。订阅、读写、AI、延迟匹配和单条命令都有 panic 恢复边界；错误恢复不等于业务重试，异常对局会记录实际失败原因。

## 日志规范

基于标准库 `log/slog` 输出 JSON，同一条记录同步写控制台与文件。时间为 UTC；每条访问日志包含请求 ID、方法、路径、状态码、响应字节数和耗时。WebSocket 包含连接 ID、用户 ID，房间事件补充房间号，走子记录起止格、步数与下一位玩家。

| 环境变量 | 默认值 | 用途 |
| --- | --- | --- |
| `PENGUIN_LOG_LEVEL` | `info` | debug / info / warn / error |
| `PENGUIN_LOG_FILE` | `logs/server.log` | 相对进程工作目录 |
| `PENGUIN_LOG_MAX_SIZE_MB` | 10 | 单文件轮转阈值 |
| `PENGUIN_LOG_MAX_BACKUPS` | 5 | 备份数量上限 |
| `PENGUIN_LOG_MAX_AGE_DAYS` | 30 | 备份保存天数 |
| `PENGUIN_REQUEST_TIMEOUT` | 10s | API 请求 context 期限 |
| `PENGUIN_STARTUP_TIMEOUT` | 15s | 启动依赖检查期限 |
| `PENGUIN_SHUTDOWN_TIMEOUT` | 10s | 关闭等待期限 |

日志备份 gzip 压缩，按数量和年龄双重清理。多实例必须使用不同日志文件，不能同时写同一个轮转文件。目录以 0700 创建，文件权限 0600；启动时检查可写性。

- INFO：启动/退出、访问日志、连接/离开、匹配成功、走子与结算。
- WARN：可预期业务拒绝、异常断线、客户端消费过慢。
- ERROR：数据库/Redis/广播/结算失败、panic；panic 带调用栈。
- DEBUG：每条命令耗时、匹配队列等调试信息。

不打印请求体、Authorization、WebSocket token 查询参数或完整请求 URL。敏感属性及配置中的密码、DSN、JWT 密钥会脱敏。记录错误时附上下文，不把 `err.Error()` 直接返回客户端，不使用 `fmt.Printf` 散落输出。日志 IO 出错时仍尝试另一路输出；上线需监控磁盘容量和控制台采集。

## 验证与维护

```bash
go test -race ./...
go vet ./...
go build -o penguin-chess-server ./cmd/api
```

仓库根目录：

```bash
node --test test/game-regression.mjs
PENGUIN_BASE_URL=http://127.0.0.1:8081 node test/api-contract.mjs
PENGUIN_BASE_URL=http://127.0.0.1:8081 node test/load.mjs 2
PENGUIN_WS_URL=ws://127.0.0.1:8081/ws node test/live-game.mjs
```

`go test` 使用伪仓储、httptest 和 miniredis，不连接真实服务。`load.mjs` 属于显式集成测试，会创建测试账号和对局记录；只用于开发数据库。新增功能时为业务错误、异常分支和并发边界补测试，提交前执行格式化、race 测试、vet 和构建。
