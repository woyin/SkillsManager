// source_cache.go 管理远程 git 源的持久化缓存（~/.sm/data/sources）。
// 被 install / cache / update 共用：安装时写入，cache 命令展示/清理，
// update 刷新 tracking 模式的缓存。
//
// Input: github.com/woyin/skills-manager/internal/sourcecache
// Output: type sourceCacheMetadata, func sourceCachePaths, func writeSourceCacheMetadata, func readSourceCacheMetadata, func cachedGitSource
// Pos: 数据层-远程git源缓存
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import "github.com/woyin/skills-manager/internal/sourcecache"

// sourceCacheMetadata 保留 cmd 层的兼容别名，避免 cache/update 的展示与
// 回写逻辑了解 sourcecache 的实现细节。
type sourceCacheMetadata = sourcecache.Metadata

// sourceCachePaths 按 source+ref 派生缓存目录与元数据路径。
// ref 参与 key，因此同一仓库的不同 pin 互不覆盖。
func sourceCachePaths(source, ref string) (string, string) {
	return sourcecache.Paths(DataDir, source, ref)
}

func writeSourceCacheMetadata(path string, meta sourceCacheMetadata) error {
	return sourcecache.WriteMetadata(path, meta)
}

func readSourceCacheMetadata(path string) sourceCacheMetadata {
	meta, _ := sourcecache.ReadMetadata(path)
	return meta
}

// cachedGitSource 把远程源克隆到 DataDir/sources 下的持久目录。
// 符号链接安装因此在进程退出后仍有效，重复安装可复用同一克隆。
// offline=true 时仅命中已有缓存，绝不发起网络 clone。
func cachedGitSource(source, ref string, offline ...bool) (string, error) {
	isOffline := len(offline) > 0 && offline[0]
	result, err := sourcecache.New(DataDir).Acquire(source, ref, isOffline)
	if err != nil {
		return "", err
	}
	return result.Path, nil
}
