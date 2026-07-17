# 发布说明（v0.1.0）

> 对应开源清单 S8。仓库：https://github.com/jeeinn/ai-dev

## 打 tag 与 Release（维护者）

工作区干净且 CI 绿后：

```bash
# 1. 确认 CHANGELOG [0.1.0] 段落已写好
git checkout master
git pull

# 2. 打 annotated tag（本地）
git tag -a v0.1.0 -m "v0.1.0: first open-source release"

# 3. 推送 tag
git push origin v0.1.0

# 4. 构建预编译二进制（示例）
GOOS=linux   GOARCH=amd64 go build -o dist/gateway-linux-amd64 .
GOOS=linux   GOARCH=arm64 go build -o dist/gateway-linux-arm64 .
GOOS=windows GOARCH=amd64 go build -o dist/gateway-windows-amd64.exe .
GOOS=darwin  GOARCH=amd64 go build -o dist/gateway-darwin-amd64 .
GOOS=darwin  GOARCH=arm64 go build -o dist/gateway-darwin-arm64 .

# 5. 创建 GitHub Release（需 gh）
gh release create v0.1.0 \
  --title "v0.1.0" \
  --notes-file docs/RELEASE-v0.1.0.md \
  dist/gateway-linux-amd64 \
  dist/gateway-linux-arm64 \
  dist/gateway-windows-amd64.exe \
  dist/gateway-darwin-amd64 \
  dist/gateway-darwin-arm64
```

前端需先 `cd web && npm ci && npm run build` 再 `go build`（embed）。

## 本版本要点

- Gitea Assign 工作流 v2（analyze / coder / review）
- Internal Agent Loop + ToolPack / MCP / Skills
- OpenCode Path A（可选本机 sidecar）
- 开源资产：LICENSE、CI、CONTRIBUTING、SECURITY、config.example、.env.example

详见 [CHANGELOG.md](../CHANGELOG.md) `[0.1.0]`。
