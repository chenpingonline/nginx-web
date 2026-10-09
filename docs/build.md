# 构建与仓库职责

通用功能只修改本仓库。版本只修改根目录 VERSION。

- Linux：按 [Linux 构建说明](../linux/README.md) 执行 make build-all。
- Docker：按 [Docker 构建说明](../docker/README.md) 执行 make docker-build。
- fnOS：进入 [飞牛打包仓库](https://github.com/chenpingonline/nginx-web-fnos)，通过 core.lock 或 NGINX_WEB_CORE 使用本仓库的源码。

scripts/stage-app.sh 共享 Go 编译、Nginx 校验、mime.types 和许可证组装；scripts/package-lib.sh 共享归档与摘要逻辑。平台包只增加各自的安装文件，不保存功能源码副本。

Linux 及 Docker 默认构建完整端口支持；飞牛根据标准版/全端口版决定前端与 Go 编译参数。Go 测试需同时运行普通模式和 -tags full_ports。

修改源码后必须重新构建嵌入的 Vue 页面。三个界面模式都由同一份 web 源码生成，禁止在打包仓库维护修改后的页面副本。

Linux 的真实 systemd 验收由 GitHub Actions 的临时原生 AMD64 runner 执行；本地 ARM64 可通过隔离容器验证程序和安装文件。没有真机测试时不得声明已在真实 fnOS 或 Linux 服务器部署。
