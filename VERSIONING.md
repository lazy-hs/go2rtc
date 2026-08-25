# 版本号管理

项目版本号以根目录的 `VERSION` 文件为唯一来源，格式为 `M.m.p`。应用构建时会将该文件嵌入二进制，不再在 Go 源码中重复维护版本号。

## 变更规则

- 常规迭代（`regular`/`minor`）：次版本号加一，修订号归零。次版本号为 9 时进位到下一主版本，例如 `1.9.5 -> 2.0.0`。
- 专项迭代（`special`/`patch`）：修订号加一，例如 `1.3.0 -> 1.3.1`。
- 重大升级（`major`）：主版本号加一，次版本号和修订号归零，例如 `1.7.8 -> 2.0.0`。
- 次版本号只允许 `0-9`，不允许出现 `1.10.0`；该版本应表示为 `2.0.0`。
- 各部分仅允许非负整数，且不允许前导零。

## 使用方法

查看和校验当前版本：

```shell
go run ./cmd/version show
go run ./cmd/version check
```

仅预览下一版本，不修改文件：

```shell
go run ./cmd/version next regular
go run ./cmd/version next special
go run ./cmd/version next major
```

执行版本变更：

```shell
go run ./cmd/version bump regular
go run ./cmd/version bump special
go run ./cmd/version bump major
```

校验指定的版本迁移：

```shell
go run ./cmd/version check-transition 1.2.3 1.3.0 regular
go run ./cmd/version check-transition 1.3.0 1.3.1 special
go run ./cmd/version check-transition 1.9.5 2.0.0 regular
```

## 发布约束

发布标签必须使用 `vM.m.p` 格式并与 `VERSION` 完全一致。例如 `VERSION` 为 `2.0.0` 时，发布标签必须为 `v2.0.0`。GitHub Actions 会在构建和推送镜像前自动检查该约束。

建议发布流程：

1. 使用 `bump` 命令按迭代类型更新版本。
2. 提交 `VERSION` 变更及本次迭代内容。
3. 创建与 `VERSION` 一致的 Git 标签。
4. 推送提交和标签，触发发布构建。
