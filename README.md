# nginx-web

通用 Nginx 反向代理管理服务，提供 Go 后端、Vue 管理页面、HTTP/HTTPS 与 TCP/UDP 代理、证书与 ACME、静态文件/WebDAV、认证、限流、日志和统计。

本仓库是唯一的功能源码仓库。Linux 和 Docker 在这里构建；[nginx-web-fnos](https://github.com/chenpingonline/nginx-web-fnos) 仅保留飞牛 FPK 的宿主集成及打包脚本，按核心提交构建同一份源码。

## 安装

- [Linux 安装包与 systemd](linux/README.md)：AMD64/ARM64 的 tar.gz，独立管理员登录，非 root 服务。
- [Docker 与 Compose](docker/README.md)：独立认证、持久化卷、AMD64/ARM64。
- [飞牛 FPK](https://github.com/chenpingonline/nginx-web-fnos)：飞牛桌面网关认证、标准版和全端口版。

## 构建

需要 Go 1.26、Node.js 22、npm、Python 3.11+、tar 和 file。Nginx 静态二进制及来源资料位于 third_party/nginx；重建方式见 scripts/build-nginx.sh。

```bash
make test
make build-all       # dist/ 下的 Linux AMD64 / ARM64 包
make docker-build
make docker-test
```

直接运行 go build/test 前先执行 make frontend-build，以生成嵌入的页面。前端构建模式支持 standalone（Linux）、docker 和 fnos，权限模式支持 standard 和 full-ports；飞牛打包会设置对应参数。

应用版本唯一来源为 [VERSION](VERSION)。前端、Go 二进制和安装包从它读取，FPK 的 manifest 由打包仓库生成。

## 只维护一份代码

1. 修改本仓库的 cmd/nginx-web、internal、web 或共享构建脚本。
2. 测试并提交核心改动。
3. 在飞牛仓库执行 python3 scripts/pin-core.py /path/to/nginx-web，固定新的核心提交，再构建 FPK。

本地联调时，在飞牛仓库使用 NGINX_WEB_CORE=/path/to/nginx-web make build-arm64，可以直接构建本仓库尚未提交的代码。正式发布使用 core.lock 固定的干净提交，包中记录核心来源。

核心提交历史继承自 nginx-web-fnos。共享的 fnOS 运行适配继续保留在核心中，平台安装和生命周期脚本由飞牛打包仓库维护。

## 文档

[功能手册](docs/user-guide.md) · [功能清单](docs/features.md) · [DNS 提供商](docs/dns-providers.md) · [构建说明](docs/build.md)

源码按 GPL-3.0 分发；Nginx 与依赖的许可证及来源资料见 LICENSE、NGINX_LICENSE、NOTICE 和 THIRD_PARTY_LICENSES.md。
