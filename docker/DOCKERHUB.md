# nginx-web — Docker 部署

nginx-web 是集成 Vue 管理页面、Go 管理服务和 Nginx 的代理管理工具，支持 HTTP/HTTPS 反向代理、TCP/UDP 转发、静态站点、证书与 ACME DNS-01、代理认证、日志和配置备份。Docker 版使用独立管理员账户。

镜像支持 **Linux AMD64 / ARM64**。`latest` 和版本标签均为双架构清单，Docker 自动选择宿主架构，无需指定 `platform` 或添加架构后缀。已发布版本见 [Tags](https://hub.docker.com/r/chenpingonline/nginx-web/tags)。

## 快速部署

在同一个部署目录保存以下 `compose.yaml`，创建管理员密码文件，再执行启动命令。此配置使用 bridge 网络，数据持久化到命名卷，支持 Linux 与 NAS 的 Docker Compose。

### compose.yaml

```yaml
name: nginx-web-fnos

services:
  nginx-web:
    image: ${NGINX_WEB_IMAGE:-chenpingonline/nginx-web:latest}
    init: false # The image already uses tini to reap Nginx child processes.
    restart: unless-stopped
    stop_grace_period: 30s
    ports:
      - "8080:8080" # Management UI; use HTTPS at your external reverse proxy.
      - "80:80"
      - "443:443"
      - "9080:9080" # Initial default HTTP listener.
      # Add matching mappings for other HTTP/TCP/UDP ports created in the UI.
      # - "5353:5353/udp"
    environment:
      FNPROXY_ADMIN_USER: admin
      FNPROXY_ADMIN_PASSWORD_FILE: /run/secrets/admin_password
      # FNPROXY_COOKIE_SECURE: "1" # Enable when accessing management via HTTPS.
      # FNPROXY_ALLOWED_PATHS: /mnt/www
    secrets:
      - admin_password
    volumes:
      - nginx-web-data:/data
      # - ./www:/mnt/www:ro
    read_only: true
    tmpfs:
      - /tmp:rw,nosuid,nodev,size=256m,mode=1777
      - /run/nginx-web:rw,nosuid,nodev,size=16m,uid=10001,gid=10001,mode=0750
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    sysctls:
      net.ipv4.ip_unprivileged_port_start: "0"

secrets:
  admin_password:
    file: ./secrets/admin_password.txt

volumes:
  nginx-web-data:

```

项目名保留为 `nginx-web-fnos`，用于兼容仓库拆分前的 Compose 数据卷。已有部署应继续使用原项目名、密码文件和数据卷；此前使用自定义 `-p` 或 `COMPOSE_PROJECT_NAME` 时继续沿用。

### 准备密码并启动

以下密码生成命令仅用于首次部署；已有部署保留原文件。

```bash
mkdir -p secrets
chmod 700 secrets
openssl rand -base64 24 > secrets/admin_password.txt
chmod 444 secrets/admin_password.txt
docker compose pull
docker compose up -d
```

打开 `http://NAS的IP:8080`，账号为 `admin`，密码为 `secrets/admin_password.txt` 的内容。可在本机使用 `cat secrets/admin_password.txt` 查看。自定义密码须为 8–72 字节，文件末尾允许一个换行。目录权限 700 限制其他宿主用户访问，文件权限 444 允许容器 UID 10001 读取 Compose Secret。

没有默认密码。首次初始化只保存 bcrypt 摘要；已有数据卷启动时提供新密码会重设密码。只重启不会更新被替换的 Secret 文件挂载，修改密码文件后使用 `docker compose up -d --force-recreate`。

### 固定版本

默认使用 `latest`。需要固定版本或回滚时，在部署目录保存 `.env`：

```dotenv
NGINX_WEB_IMAGE=chenpingonline/nginx-web:0.3.12
```

替换为 [Tags](https://hub.docker.com/r/chenpingonline/nginx-web/tags) 中需要的已发布版本，再执行相同的拉取与启动命令。

## 端口与代理规则

默认发布管理端口 `8080`、HTTP/HTTPS 代理端口 `80`、`443` 和初始默认 HTTP 端口 `9080`。没有规则时 `9080` 返回 404。管理端口须独占，不能同时作为代理规则的监听端口。

在页面添加 HTTP 规则，设置监听端口与域名，填写容器可达的后端，再保存并应用。HTTPS 规则须先导入证书或配置 ACME DNS-01。同一 Docker 网络的后端可使用服务名，NAS 或局域网服务使用容器可达的 IP；容器中的 `127.0.0.1` 指向自身。

宿主端口已被占用时，修改映射左侧，例如 `18080:8080` 或 `9081:80`，页面访问地址同步调整。新增代理端口时同时增加 Compose 映射，UDP 须指定 `/udp`，例如 `5353:5353/udp`。镜像的 `EXPOSE` 不会自动发布端口。

容器以 UID/GID `10001:10001` 运行，根文件系统只读，移除全部 capabilities。Compose 中的 `net.ipv4.ip_unprivileged_port_start=0` 允许 bridge 容器内监听低端口，无需特权模式或 Docker Socket。

## 数据持久化与备份

`nginx-web-data` 命名卷挂载到 `/data`，保存配置、草稿、证书与私钥、ACME 账户和 DNS 凭据、独立管理员账户摘要及日志。一个数据卷只应由一个运行中的实例使用。

启动恢复已应用配置，保留未应用草稿。页面 JSON 备份包含代理、证书和设置，但不包含独立管理员账户、ACME 账户、DNS 凭据或续期任务。完整备份应停止容器后备份 `/data` 与外部挂载目录。

若改用宿主目录挂载，先创建该目录并赋予 UID/GID `10001:10001` 所有权，再替换 Compose 的数据卷挂载。

## 外部静态目录

在 Compose 服务的 `environment` 与 `volumes` 中增加：

```yaml
environment:
  FNPROXY_ALLOWED_PATHS: /mnt/www
volumes:
  - nginx-web-data:/data
  - ./www:/mnt/www:ro
```

保留原管理员密码变量和其他配置。在页面填写容器路径 `/mnt/www`。宿主目录需要 UID 10001 可遍历、可读取；WebDAV 需要写权限，应使用可写挂载。只授权实际需要的目录，多个允许目录以冒号分隔。

## 升级、停止与管理 HTTPS

保留原密码文件、项目名和数据卷；使用固定版本时先修改 `.env` 的 `NGINX_WEB_IMAGE`。

```bash
docker compose pull
docker compose up -d
```

`docker compose stop` 或 `docker compose down` 正常停止服务并保留数据。`docker compose down -v` 会删除命名卷中的数据。

管理入口通过 HTTPS 反向代理访问时，在服务环境变量中设置 `FNPROXY_COOKIE_SECURE: "1"`，让外层代理保留原始 Host。管理登录使用会话 Cookie，写 API 使用 CSRF token。

## 项目与文档

- [核心源码](https://github.com/chenpingonline/nginx-web)
- [完整 Docker 部署说明](https://github.com/chenpingonline/nginx-web/blob/master/docker/README.md)
- [使用指南](https://github.com/chenpingonline/nginx-web/blob/master/docs/user-guide.md)
- [飞牛 FPK 版](https://github.com/chenpingonline/nginx-web-fnos)

Docker 版使用独立认证，不包含 WAF 实验功能。Docker Hub Overview 提供可复制的部署配置；`docker pull` 只下载镜像，不下载 Compose 或密码文件。
