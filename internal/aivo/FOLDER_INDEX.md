# internal/aivo 文件夹索引

## 架构说明
业务集成层，封装与 aivo 服务的对接逻辑。
独立模块，无内部包依赖，被 cmd/status 等命令调用以读取 aivo 相关状态。

## 文件清单

### aivo.go
- **地位**: aivo 集成的唯一实现
- **功能**: 读取/解析 aivo 的 token 用量与状态信息
- **依赖**: （无内部包依赖）
- **被依赖**: cmd/status（健康检查展示 aivo 状态）

### aivo_test.go
- **功能**: 固定payload与执行seam覆盖解析、精确参数、非零退出、非法JSON、超时与nil兼容；默认不调用宿主 aivo

### aivo_integration_test.go
- **功能**: opt-in 真实 aivo 冒烟（仅 `SM_AIVO_INTEGRATION=1` 运行，默认 skip）

---
⚠️ **自指声明**: 当本文件夹内容变化时，请更新此索引
