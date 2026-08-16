# 原版 Immich 并排对比（Docker side-by-side）

目标：把**原版 Immich v3.1.0** 与本项目 **immich-go** 放在同一台有 Docker 的
机器上，对照**官方客户端与网页版实际代码**（immich 仓库 `server/`、`web/`、
`packages/sdk/`，v3.1.0）逐 endpoint 对比一致性，验证 immich-go 的兼容覆盖与
DTO 形状是否与原版对齐；OpenAPI 规范（v3.1.0）仅作路径/方法清单参考，不作为
"为准"的最终依据。

> ⚠️ 本仓库的开发沙箱**没有 CAP_SYS_ADMIN**，无法启动 `dockerd`，因此下列
> 步骤需在**具备 Docker 的主机**（或本机）执行；本目录仅提供可复用配置与脚本。

## 前置
- Docker + docker compose v2（`docker compose version` 可见）。
- Python3 + `pip install schemathesis==4.24.3`（`scripts/compare_origins.py` 用）。
- 已构建 immich-go（`./immich-go`，默认 `:8081`）。

## 步骤
1. 准备环境变量（原版 Immich 的 compose 用 `env_file: .env`）：
   ```bash
   cp .env.example .env
   # 按需改密码；默认 admin@immich.app / password，与 immich-go 默认管理员一致
   ```
2. 启动原版 Immich（postgres + redis + server + machine-learning），端口 `2283`：
   ```bash
   docker compose -f docker-compose.yml up -d
   # 首次需建管理员：浏览器打开 http://localhost:2283 完成初始化
   # （若服务器支持 IMMICH_ADMIN_EMAIL/PASSWORD 引导，也可直接由 compose 注入）
   ```
3. 启动 immich-go（另开终端，端口 `8081`，默认管理员同 creds）：
   ```bash
   ./immich-go            # http://localhost:8081
   ```
4. 并排对比：
   ```bash
   python3 scripts/compare_origins.py \
       --origin-url http://localhost:2283/api \
       --go-url     http://localhost:8081/api
   ```
   脚本对两端各跑一次 Schemathesis（`--phases examples`），解析两份 JUnit，
   按 endpoint 打印 `ORIGIN / IMMICH-GO / NOTE`：
   - `OK`：两端都通过（immich-go 覆盖且与契约一致）。
   - `GAP`：原版通过、immich-go 未通过/未覆盖（实现缺口，对应 STATUS.md §K 的已知限制）。
   - `BOTH_FAIL`：两端都失败（多为该端点两边都未实现/契约歧义）。
   - `GO-AHEAD`：immich-go 通过但原版未通过（少见，多为原版该端点需特定前置数据）。

## 预期结论
原版 Immich 是契约的参考实现，应大面积 `OK`；immich-go 在其已实现子集上应同样
`OK`，未实现端点显示为 `GAP`。若某已实现 endpoint 出现 `GAP` 且失败类型为
`schema`/`content_type`/`5xx`，即 DTO 形状回退——应回到
`scripts/schemathesis_check.py` 回归门禁排查（见 `docs/CONTRACT_TESTING.md`）。

## 说明 / 限制
- 原版 Immich 的 machine-learning 服务在此对比中关闭 GPU（`DISABLE_GPU=true`），
  因为并排只关心 **API 面**，ML 推理不影响契约对比。
- immich-go 与原版共用同一份 `open-api/immich-openapi-specs.json`（v3.1.0），
  保证对比 apples-to-apples。
- 本对比不改写任何服务的状态；如需用真实资产数据对比，先在原版里上传若干
  图片/视频，再在 immich-go 侧用相同账号查看，肉眼核对时间线/相册/播放。
