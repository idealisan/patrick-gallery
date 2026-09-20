# API 契约一致性回归测试（Schemathesis）

immich-go 的兼容基准是**官方客户端与网页版实际代码**（immich 仓库 `server/`、`web/`、`packages/sdk/`，tag **v3.1.0**）。本项目用行业标准 property-based API 测试工具 **[Schemathesis](https://schemathesis.readthedocs.io/)** 对运行中的 immich-go 做真实契约一致性核验，并将其作为**回归测试手段**：任何让响应体形状（DTO）偏离契约的改动都会被 CI 拦下。[官方 Immich v3.1.0 OpenAPI 规范](https://github.com/immich-app/immich/blob/v3.1.0/open-api/immich-openapi-spec.json)仅作为 Schemathesis 校验所用的 schema 来源，**不是契约权威**。

历史运行报告见 [`../reports/schemathesis-v3.1.0.md`](../reports/schemathesis-v3.1.0.md)；原始产物（JUnit + 文本）在 [`../reports/schemathesis/`](../reports/schemathesis/)。

## 为什么不是手写检查器

早期用 `scripts/api_consistency.py`（stdlib-only 的 GET 检查器）做只读一致性调查，但它只能“猜”响应体是否合法、且会漏报 DTO 形状错误（例如把 `{"markers":[...]}` 当成合法、把平行数组对象当成合法）。Schemathesis 直接拿 **OpenAPI schema**（仅作回归校验输入）校验每个响应，能精确报告 `response_schema_conformance` / `content_type_conformance` / `status_code_conformance` / `not_a_server_error` 四类违例——这正是修复 A1–A4（见 STATUS.md §K）的关键手段。两者互补：`api_consistency.py` 覆盖 75 个 GET 的广撒网，Schemathesis 对可生成样例的 operation 做严格 schema 校验。权威契约仍以官方客户端与网页版实际代码为准。

## 回归门禁脚本

`scripts/schemathesis_check.py` 是自包含的回归门禁：

1. `CGO_ENABLED=0` 构建 immich-go；
2. 在临时库（`IMMICH_COMPAT_VERSION=3.1.0`）启动服务，等 `/api/server/ping`；
3. 用管理员账号登录拿 token；
4. 运行 Schemathesis（`--phases examples`，默认 `--max-examples 1`）并写出 JUnit 报告；
5. 解析 JUnit，按检查类型分类失败并裁决。

### 裁决逻辑（关键）

| 检查 | 类别 | 门禁 |
|------|------|------|
| `response_schema_conformance` | 关键 | **>0 即失败**（DTO 形状错误） |
| `content_type_conformance` | 关键 | **>0 即失败** |
| `not_a_server_error` (5xx) | 关键 | **>0 即失败** |
| `status_code_conformance` | 允许 | 仅允许出现在 `scripts/schemathesis-allowlist.txt` 中的 operation；出现未列出的 operation → 失败（回归） |

设计意图：今天关键检查全绿、status_code 缺口全部在 allowlist 中，门禁通过；一旦未来改动破坏了某个已实现端点的响应形状，或让某个原本正常的端点开始返回未声明状态码，门禁立即变红。`status_code_conformance` 的缺口绝大多数是**尚未实现的端点（返回 404）**与本就存在的良性边界（`POST /assets` 400 负向、`POST /auth/change-password` 204、`POST /auth/login` 401 工具鉴权伪影），这些不是 DTO 形状问题，故用 allowlist 显式豁免。

## 如何运行

```bash
# 自动构建 + 启动 + 测试（默认端口 8099，max-examples 1）
python3 scripts/schemathesis_check.py

# 指向已运行的实例（跳过构建/启动）
python3 scripts/schemathesis_check.py --no-server --base-url http://localhost:8081/api --token "$TOKEN"

# 调高样例数以加强覆盖（更慢）
python3 scripts/schemathesis_check.py --max-examples 5
```

依赖：Go（构建）、Python3 + `schemathesis`（首次运行会自动 `pip install schemathesis==4.27.5`）。

退出码：`0` = 通过；`1` = 检测到契约回归。

## 基线（v1.3.1-go，a14c98d）

以官方 v3.1.0 spec 实跑（Schemathesis 4.27.5，`--phases examples`）：

- Tested 30 / 254 operation；Passed **5**；Failed **25**。
- 关键检查（schema / content-type / 5xx）：**全部 0**。
- status_code 缺口 25 个，全部落在 `schemathesis-allowlist.txt`（即未实现端点 + 3 个良性边界）。
- 修复前对比：响应违反 schema **7 → 0**，通过 **1 → 5**（见报告）。

## CI 集成

`.github/workflows/ci.yml` 新增 `contract-test` job：每次 push 到 `main` 与 PR 时 checkout → setup-go → `pip install schemathesis==4.27.5` → 运行 `scripts/schemathesis_check.py`。该 job 失败即阻断合并，确保 DTO 形状不回退。CNB 流水线（`.cnb.yml`）亦可复用同一脚本作为发版前门禁。

## 维护 allowlist

当你**实现**了某个此前未实现的端点（或修掉了某个良性 4xx 边界），把它从 `scripts/schemathesis-allowlist.txt` 删除并重新运行脚本即可——该 operation 的 `status_code_conformance` 若仍失败但已不在 allowlist 中，门禁会失败提醒你补测；若已通过则门禁继续保持绿。新增 operation 时无需改动 allowlist（脚本只因“未列出的 status_code 失败”才失败）。

> 不要把“为掩盖回归而往 allowlist 加行”当做法：allowlist 仅用于已知且**有意**未实现的端点/良性边界；真实 DTO 形状错误必须由关键检查拦下，不允许进 allowlist。
