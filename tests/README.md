# Tests

- make test: Vue 构建与 Go standard/full_ports 测试。
- tests/integration.sh: Linux 上的私有 Socket、HTTP/HTTPS 和 Nginx 集成。
- python3 docker/tests/integration.py --image nginx-web:docker: 实际容器认证、CSRF、代理、静态授权、备份、重启和优雅退出。
- sudo python3 linux/tests/service.py: 仅用于临时 CI runner 的原生 systemd、身份/能力、低端口代理和重启检查；不可在生产服务器执行。

飞牛生命周期、恢复诊断和低端口测试位于 nginx-web-fnos 仓库。
