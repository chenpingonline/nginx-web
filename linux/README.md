# Linux 安装包

支持 AMD64 和 ARM64，安装包包含管理服务、静态 Nginx、许可证、systemd 服务及安装/卸载脚本。应用版本只取自仓库根目录的 `VERSION`。

## 安装

需要使用 systemd 的 Linux 系统，以及 Bash、Python 3（源码构建/测试时）、`openssl`、`file`、`sha256sum` 和 `useradd`。以 AMD64 的 0.3.9 包为例：

```bash
sha256sum -c nginx-web-0.3.9-linux-amd64.tar.gz.sha256
mkdir nginx-web-install
tar -xzf nginx-web-0.3.9-linux-amd64.tar.gz -C nginx-web-install
cd nginx-web-install
sudo ./install.sh
sudo systemctl status nginx-web
```

ARM64 对应使用 `linux-arm64` 包。安装前会校验包内文件摘要和二进制架构。安装器保留已有账号密码、环境配置和应用数据。`--no-start` 仅安装文件，适合制作系统镜像；运行服务需要 systemd。

管理入口默认为 `http://服务器地址:8080`，管理员为 `admin`。首次安装生成随机密码，使用 `sudo cat /etc/nginx-web/admin_password` 在服务器上查看；安装日志不会打印密码。Nginx 的初始 HTTP 端口为 9080，无规则时返回 404。

## 配置与数据

| 路径 | 内容 |
| --- | --- |
| `/usr/local/lib/nginx-web` | 程序、Nginx 与许可证 |
| `/etc/nginx-web/nginx-web.env` | systemd 环境配置 |
| `/etc/nginx-web/admin_password` | 初始化/重设密码的文件 |
| `/var/lib/nginx-web/etc` | 已应用的 Nginx 配置与历史 |
| `/var/lib/nginx-web/var` | 草稿、管理员摘要、证书、ACME、日志及统计 |
| `/run/nginx-web` | 私有 Socket 和临时数据 |

修改环境配置或密码文件后执行 `sudo systemctl restart nginx-web`。提供新的密码文件内容会在启动时更新管理员摘要，重启使旧登录会话失效。移除密码文件变量后可以继续使用已保存的摘要。

管理入口放在 HTTPS 反向代理后时，设置 `FNPROXY_COOKIE_SECURE=1` 并保留原始 Host。限制为本机访问可设置 `FNPROXY_LISTEN=127.0.0.1:8080`。

静态目录、外部认证文件和客户端 CA 需要通过 `FNPROXY_ALLOWED_PATHS=/srv/www:/srv/auth` 授权。目录必须允许 `nginx-web` 用户遍历和读取；WebDAV 还需要写权限。systemd 默认阻止访问 `/home`，请将需要的服务文件放在 `/srv` 等目录。写入 WebDAV 目录还需在 systemd override 的 `ReadWritePaths` 中明确加入目录。

服务以 `nginx-web` 用户运行，仅授予绑定低端口的 `CAP_NET_BIND_SERVICE`，不以 root 身份运行管理服务。保存未应用草稿后重启，会恢复最后一次已应用配置并保留草稿。

## 升级与卸载

下载新版本后重新执行安装步骤，已有配置、密码和数据会保留。升级前停止服务并备份 `/etc/nginx-web`、`/var/lib/nginx-web` 及外部服务目录。JSON 备份用于迁移代理配置；完整备份还需包含管理员摘要、ACME 账户和 DNS 凭据。

```bash
sudo ./uninstall.sh
```

卸载仅移除程序及 systemd 服务，保留账号、配置和数据。

## 源码构建

在项目根目录执行：

```bash
make test
make build-all
```

输出为 `dist/nginx-web-<version>-linux-{amd64,arm64}.tar.gz` 和各包的 `.sha256`。Linux 与飞牛打包调用同一个 `scripts/stage-app.sh`，共享 Go/Vue 源码和 Nginx。

核心仓库的 CI 在临时 Linux runner 上验证实际 systemd 安装、非 root 运行、HTTP/HTTPS 低端口、认证、重启和卸载保留。容器内验证不等于真实服务器或飞牛安装验证。
