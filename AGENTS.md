# nginx-web 核心仓库

- 通用 Go 后端、Vue 前端、Nginx 配置生成、Linux 与 Docker 构建只在本仓库维护。
- 飞牛的清单、安装升级、权限修复和 FPK 组装位于 `chenpingonline/nginx-web-fnos`，不要复制平台安装代码到核心仓库。
- 保持核心的 fnOS 与 standalone 运行适配兼容。界面构建模式为 fnos、standalone（Linux）或 docker。
- 应用版本唯一来源为根目录 `VERSION`；前端、Go 程序和各平台安装包必须从它读取。
- 修改共享行为时验证 standard 和 full_ports；前端构建验证对应平台。可访问运行界面时用 Computer Use 检查受影响流程。
- 不修改生产 Nginx、systemd、Docker 或防火墙来运行测试。Linux 安装测试使用临时容器或 CI runner。
