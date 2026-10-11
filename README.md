<div align="center">

<img src="https://raw.githubusercontent.com/chenpingonline/nginx-web-fnos/master/packaging/fnos/ICON_256.PNG" alt="nginx-web" width="128" />

# nginx-web

**面向 Linux、Docker 与飞牛 fnOS 的 Nginx 可视化管理器**

[![Build](https://github.com/chenpingonline/nginx-web/actions/workflows/build.yml/badge.svg?branch=master)](https://github.com/chenpingonline/nginx-web/actions/workflows/build.yml)
![Linux](https://img.shields.io/badge/Linux-AMD64%20%7C%20ARM64-2ea44f)
[![NGINX](https://img.shields.io/badge/Core-NGINX%201.30.4-009639)](https://nginx.org/)
[![License](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)

[Linux 安装](linux/README.md) · [Docker 部署](docker/README.md) · [飞牛版](https://github.com/chenpingonline/nginx-web-fnos) · [使用指南](docs/user-guide.md) · [问题反馈](https://github.com/chenpingonline/nginx-web/issues)

</div>

<p align="center">
  <img src="docs/images/dashboard-light.png" alt="nginx-web 管理界面示例：飞牛版的运行状态、实时流量与代理规则" width="100%" />
</p>

图示来自飞牛版；Linux、Docker 和飞牛版共享代理、证书与监控页面，管理入口和认证方式按平台适配。

## 项目简介

通用 Nginx 反向代理管理服务，提供 Go 后端、Vue 管理页面、HTTP/HTTPS 与 TCP/UDP 代理、证书与 ACME、静态文件/WebDAV、认证、限流、日志和统计。

本仓库是唯一的功能源码仓库。Linux 和 Docker 在这里构建；[nginx-web-fnos](https://github.com/chenpingonline/nginx-web-fnos) 仅保留飞牛 FPK 的宿主集成及打包脚本，按核心提交构建同一份源码。

- **可视化配置**：通过结构化表单管理域名、端口、路径、证书、负载均衡与访问控制。
- **独立 Nginx**：内置 Nginx Open Source 1.30.4，使用自己的进程、配置与日志目录。
- **多种部署方式**：Linux 使用 systemd，Docker 使用持久化卷，飞牛使用桌面统一网关。
- **配置校验与恢复**：应用前执行 `nginx -t`，失败时尝试恢复上一份有效配置；保存的草稿不会在重启时自动生效。

## 功能

| 模块 | 功能 |
| --- | --- |
| 总览 | 查看 Nginx 运行状态、HTTP 请求趋势、连接数、错误率及规则生效状态 |
| 代理 HTTP(S) | 管理域名与路径转发、WebSocket、SSE、HTTP/2、静态文件、WebDAV 和跳转 |
| 规则分组 | 共享监听类型、协议、监听端口、证书与 HTTP/2 默认值，支持逐项取消继承 |
| TCP/UDP 代理 | 四层转发、TLS 终止、SNI 分流、PROXY Protocol 与访问控制 |
| 转发服务组 | 管理多个节点、权重、备用节点及负载均衡策略，可供多个规则复用 |
| 限流策略 | 管理请求速率、突发请求、并发连接与下载速度限制 |
| SSL/TLS 证书 | 导入 PEM 证书，通过 ACME DNS-01 自动签发与续期，接入 39 个 DNS 服务商适配器 |
| 请求详情 | 按时段与规则查看 HTTP 请求、4xx/5xx 错误趋势和统计覆盖情况 |
| 运行日志 | 查看 HTTP、Stream、Nginx 错误及管理服务日志 |
| 备份与恢复 | 导出 JSON 备份，恢复代理、分组、证书和设置为草稿 |
| 配置历史 | 保存配置快照、查看历史并恢复为草稿 |
| Nginx 配置 | 查看生成配置，管理独立自定义配置，执行校验并应用草稿 |
| 全局设置 | 管理默认端口、Worker、TLS、Gzip、Real IP、日志轮转和缓存等参数 |

首次配置与排查请看 [使用指南](docs/user-guide.md)，详细配置项见 [功能与使用说明](docs/features.md)，DNS 凭据配置见 [DNS 服务商说明](docs/dns-providers.md)。共用手册中的飞牛安装示例应结合下面的平台部署文档阅读。

## Nginx 工作方式

Go 管理服务提供页面与 API，独立 Nginx 处理实际代理流量：

```text
Linux / Docker 管理入口：浏览器 → 管理 HTTP 端口 → Go 管理服务
飞牛管理入口：fnOS 桌面 → 统一网关 → Unix Socket → Go 管理服务
代理流量：客户端 → 应用内置 Nginx → NAS / 容器 / 局域网服务
```

1. 通过表单创建或修改配置，保存为草稿。
2. 应用时生成候选配置，并执行 `nginx -t` 校验。
3. 校验通过后替换正式配置；Nginx 运行时执行平滑重载。
4. 启动或重载失败时，尝试恢复上一份有效配置。

恢复备份或历史快照会生成草稿，检查后再应用。界面上的“保存并应用”会应用全部待生效修改。

## 支持平台

| 部署方式 | 架构与产物 | 管理认证及端口 |
| --- | --- | --- |
| Linux systemd | AMD64 / ARM64 的 `linux-amd64.tar.gz` / `linux-arm64.tar.gz` | 独立管理员登录，默认管理端口 8080；服务获专用低端口绑定权限 |
| Docker / Compose | `linux/amd64` / `linux/arm64` 镜像 | 独立管理员登录，默认发布管理端口 8080；代理端口需配置映射 |
| 飞牛 fnOS | 独立打包仓库提供 x86_64 / ARM64 FPK | 飞牛网关身份；标准版监听 1024–65535，全端口版监听 1–65535 |

三种部署方式使用同一套功能代码。外部目录授权、宿主权限和服务管理按平台处理，详情见各自的安装文档。

## 安装

- [Linux 安装包与 systemd](linux/README.md)：AMD64/ARM64 的 tar.gz，独立管理员登录，非 root 服务。
- [Docker 与 Compose](docker/README.md)：独立认证、持久化卷、AMD64/ARM64。
- [飞牛 FPK](https://github.com/chenpingonline/nginx-web-fnos)：飞牛桌面网关认证、标准版和全端口版。

### Linux

从 [成功的 CI 构建](https://github.com/chenpingonline/nginx-web/actions/workflows/build.yml) 获取安装包，或按下文从源码构建。将对应架构的包和校验文件复制到目标 Linux 服务器。以 AMD64 的 0.3.9 包为例：

```bash
sha256sum -c nginx-web-0.3.9-linux-amd64.tar.gz.sha256
mkdir nginx-web-install
tar -xzf nginx-web-0.3.9-linux-amd64.tar.gz -C nginx-web-install
cd nginx-web-install
sudo ./install.sh
sudo systemctl status nginx-web
```

打开 `http://服务器地址:8080`，账号为 `admin`，首次生成的密码可在服务器上通过 `sudo cat /etc/nginx-web/admin_password` 查看。升级、数据保留和外部目录授权见 [Linux 安装说明](linux/README.md)。

### Docker

Docker Hub 镜像 [chenpingonline/nginx-web](https://hub.docker.com/r/chenpingonline/nginx-web) 的版本标签和 `latest` 均支持 AMD64、ARM64，Docker 自动选择架构。在本仓库的 `docker/` 目录准备管理员密码并启动：

```bash
cd docker
mkdir -p secrets
chmod 700 secrets
openssl rand -base64 24 > secrets/admin_password.txt
chmod 444 secrets/admin_password.txt
docker compose pull
docker compose up -d --no-build
```

打开 `http://宿主地址:8080`，账号为 `admin`，密码使用上述文件内容。此初始化示例适用于首次部署；已有部署应继续使用原密码文件和数据卷。固定版本可在 `docker/.env` 设置 `NGINX_WEB_IMAGE=chenpingonline/nginx-web:0.3.12`。源码构建、离线镜像加载、端口映射、目录挂载和升级见 [Docker 部署说明](docker/README.md)。

### 飞牛 fnOS

在 [nginx-web-fnos Releases](https://github.com/chenpingonline/nginx-web-fnos/releases/latest) 下载符合 CPU 架构和权限模式的 FPK，通过 **fnOS → 应用中心 → 手动安装** 安装，再从飞牛桌面打开 nginx-web。完整的平台要求、安装与升级步骤见 [飞牛版 README](https://github.com/chenpingonline/nginx-web-fnos/blob/master/README.md)。

## 快速上手

以将 `nas.example.com:9080` 转发到局域网服务 `192.168.1.10:3000` 为例：

1. 确保 Nginx 所在服务器或容器能访问目标服务，并将域名解析到可访问的入口地址。
2. 打开 **HTTP(S) 代理**，添加 HTTP 规则：域名填 `nas.example.com`，监听端口填 `9080`，监听类型按实际网络选择 IPv4、IPv6 或双栈。
3. 后端协议选 HTTP，主机填 `192.168.1.10`，端口填 `3000`；按需开启 WebSocket 或流式传输。
4. 保存并应用，在总览确认规则已生效、Nginx 正在运行，再访问 `http://nas.example.com:9080`。

Docker 需发布对应代理端口；容器内的 `127.0.0.1` 指向本容器，目标服务应使用容器可达的地址或同一 Docker 网络内的服务名。管理端口与代理监听端口必须分开。

需要 HTTPS 时，先导入 PEM 证书或通过 ACME DNS-01 申请，再选择证书和可用监听端口。通配符证书与 DNS 凭据配置见 [DNS 服务商说明](docs/dns-providers.md)。

## 配置与数据

| 部署方式 | 持久化位置 | 升级及备份 |
| --- | --- | --- |
| Linux | `/etc/nginx-web`、`/var/lib/nginx-web` | 重新执行安装器保留配置、密码与数据；完整备份前先停止服务 |
| Docker | `/data` 数据卷及外部挂载目录 | 重建容器保留数据卷；原 Compose 项目名继续使用，避免切换到新卷 |
| 飞牛 | fnOS 提供的应用配置和数据目录 | 手动安装新版 FPK；卸载向导默认保留应用数据 |

JSON 备份包含代理、分组、证书与设置，恢复后为草稿；它包含证书私钥，但不包含 ACME 账户、DNS 凭据或自动续期任务，独立管理员账户也需按平台单独处理。完整迁移还需备份持久化目录与外部文件，恢复后检查路径和授权再应用。

## 项目结构

```text
nginx-web/
├── cmd/nginx-web/        # Go 管理服务入口
├── internal/             # 配置模型、API、证书、统计和 Nginx 管理
├── web/                  # 共用 Vue 3 + TypeScript 管理页面
├── third_party/nginx/    # 分架构 Nginx、源码来源与许可证资料
├── linux/                # Linux 安装、systemd 和服务测试
├── docker/               # Dockerfile、Compose 和镜像集成测试
├── scripts/              # 共用程序组装、归档、版本与 Nginx 构建
├── tests/                # 共用 Unix Socket 集成测试
├── docs/                 # 功能、使用与构建说明
├── VERSION               # 唯一的应用版本来源
└── dist/                 # Linux 包及校验文件，不提交
```

飞牛清单、桌面入口、权限修复与生命周期脚本位于独立的 [nginx-web-fnos](https://github.com/chenpingonline/nginx-web-fnos) 仓库；Docker 离线镜像输出到 `docker/dist/`。

## 构建

需要 Go 1.26、Node.js 22.12+、npm、Python 3.11+、Git、tar 和 `file`。Nginx 静态二进制及来源资料位于 `third_party/nginx`；重建方式见 `scripts/build-nginx.sh`。

```bash
git clone https://github.com/chenpingonline/nginx-web.git
cd nginx-web
make test
make build-all       # dist/ 下的 Linux AMD64 / ARM64 包
make docker-build
make docker-test
```

直接运行 `go build/test` 前先执行 `make frontend-build`，以生成嵌入的页面。前端构建模式支持 `standalone`（Linux）、`docker` 和 `fnos`，权限模式支持 `standard` 和 `full-ports`；飞牛打包会设置对应参数。多个构建若使用同一个核心工作区，应顺序执行，避免覆盖彼此的 `web/dist`。

`make test` 构建页面并运行普通模式与 `full_ports` 的 Go 测试。`make build-all` 生成 Linux 双架构包和校验文件。Docker 构建与测试需要 Docker、BuildKit 和 Compose；跨架构 Nginx 编译还需对应架构的构建节点或模拟器，详情见 [构建说明](docs/build.md)。

应用版本唯一来源为 [VERSION](VERSION)。前端、Go 二进制和安装包从它读取，FPK 的 manifest 由打包仓库生成。

## 只维护一份代码

1. 修改本仓库的 cmd/nginx-web、internal、web 或共享构建脚本。
2. 测试、提交并推送核心改动到 `master`。
3. 在飞牛仓库执行 `python3 scripts/pin-core.py /path/to/nginx-web`，固定新的核心提交，再构建 FPK。

本地联调时，在飞牛仓库使用 `NGINX_WEB_CORE=/path/to/nginx-web make build-arm64`，可以直接构建本仓库尚未提交的代码。正式发布使用 `core.lock` 固定的干净提交，包中记录核心来源。

核心提交历史继承自 nginx-web-fnos。共享的 fnOS 运行适配继续保留在核心中，平台安装和生命周期脚本由飞牛打包仓库维护。

## 常见问题

### 会影响系统已有的 Nginx 吗？

应用使用自己的 Nginx、配置、数据与临时目录。新代理监听端口仍需避开已有服务；Docker 发布的宿主端口也不能与其他服务冲突。

### 80/443 端口需要 root 运行应用吗？

Linux 的 systemd 服务以 `nginx-web` 用户运行，仅授予 `CAP_NET_BIND_SERVICE`。Docker 使用非 root 用户和独立网络命名空间的低端口设置。飞牛标准版不能监听低端口，全端口版在生命周期阶段设置 Nginx 的专用能力后降权。具体权限边界见各平台部署文档。

### 可以直接编辑 nginx.conf 吗？

系统生成配置保持只读，常规修改通过结构化表单完成；高级用户可在 Nginx 配置页管理独立的 `custom-*.conf` / `custom-*.stream` 文件，并在应用前执行完整校验。

### 为什么没有流量数据？

统计从开始采集后积累；缺失历史保留为空。关闭访问日志会停止规则日志统计。WebSocket、SSE 和长下载在请求结束后才计入完成请求数。TCP/UDP 会话不计入 HTTP 指标。

### 支持哪些证书验证方式？

ACME 当前支持 DNS-01，尚不支持 HTTP-01、IP 地址签发或飞牛系统证书同步。已接入适配器不代表每个 DNS 服务商都经过真实账号验证。

### 为什么首次打开代理端口返回 404？

没有配置规则时，应用的默认 `9080` 端口返回 404。添加匹配域名与端口的代理规则并应用后，再使用该域名访问；管理页面使用单独的管理入口。

## 当前限制

内置 Nginx 未包含 HTTP/3/QUIC、Brotli、Lua/OpenResty、JWT、headers-more、GeoIP2、ModSecurity/WAF、第三方主动健康检查或 Prometheus 模块。

Basic Auth 文件、客户端 CA、静态目录等外部路径必须显式授权并允许应用用户访问。Linux 的服务目录权限与 systemd 写入范围、Docker 挂载及端口映射、飞牛访问权限分别在部署文档中说明。

CI 验证通用 Go/Vue、原生 AMD64 Linux systemd 和 Docker 集成；飞牛的权限与生命周期测试在打包仓库。构建、容器及 CI 测试不能替代目标服务器或实体 fnOS 设备的安装、升级与使用验证。

## 开发与贡献

欢迎通过 [Issue](https://github.com/chenpingonline/nginx-web/issues) 反馈通用功能问题，或提交 Pull Request。飞牛安装、宿主授权与生命周期问题请提交到 [飞牛仓库](https://github.com/chenpingonline/nginx-web-fnos/issues)。

问题反馈请包含应用版本、部署方式、系统与 CPU 架构、可复现步骤、预期与实际结果，以及脱敏后的日志或截图。请勿上传私钥、DNS Token、管理员密码或完整配置备份。

## 致谢

- [NGINX](https://nginx.org/)：HTTP 与 Stream 代理核心。
- [lego](https://github.com/go-acme/lego)：ACME 证书签发与 DNS 服务商适配器。
- [Vue](https://github.com/vuejs/core) 与 [Vite](https://github.com/vitejs/vite)：管理界面与前端构建。
- [Phosphor Icons](https://github.com/phosphor-icons/vue)：界面图标。

## 文档

[功能手册](docs/user-guide.md) · [功能清单](docs/features.md) · [DNS 提供商](docs/dns-providers.md) · [构建说明](docs/build.md)

## License

项目源码采用 [GNU GPL v3](LICENSE)。Nginx 与依赖遵循各自的许可证，来源与说明见 [NGINX_LICENSE](NGINX_LICENSE)、[NOTICE](NOTICE) 和 [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md)。
