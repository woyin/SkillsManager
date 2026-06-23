#!/usr/bin/env python3
# 由 .github/workflows/release.yml 在每次发版后调用。
#
# 读取 dist/checksums.txt 中四个目标平台的 sha256,结合版本号,
# 用 string.Template 整体重生成 Formula/sm.rb。整体重生成比 sed
# 原地替换更稳健(不存在 url 与 sha256 配对错乱的风险)。
#
# 用法:python3 sync_formula.py <version> <tag> <dist_dir>
#   例:python3 sync_formula.py 0.2.0 v0.2.0 dist
import sys
import pathlib
from string import Template

if len(sys.argv) != 4:
    raise SystemExit(f"用法: {sys.argv[0]} <version> <tag> <dist_dir>")

ver, tag, dist_dir = sys.argv[1], sys.argv[2], sys.argv[3]
checksums = pathlib.Path(dist_dir, "checksums.txt").read_text()


def sha(pattern: str) -> str:
    """从 checksums.txt 中取出文件名匹配 pattern 的那一行的 sha256。"""
    for line in checksums.splitlines():
        if pattern in line:
            return line.split()[0]
    raise SystemExit(f"✗ checksums.txt 中找不到匹配 {pattern!r} 的行")


# Template 仅展开 $name / ${name};ruby 的 #{bin} 不含 $ 故原样保留。
BASE = "https://github.com/woyin/SkillsManager/releases/download"
TEMPLATE = Template("""# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "$ver"

  on_macos do
    on_arm do
      url "$base/$tag/sm_${tag}_darwin_arm64.tar.gz"
      sha256 "$darwin_arm64"
    end
    on_intel do
      url "$base/$tag/sm_${tag}_darwin_amd64.tar.gz"
      sha256 "$darwin_amd64"
    end
  end

  on_linux do
    on_arm do
      url "$base/$tag/sm_${tag}_linux_arm64.tar.gz"
      sha256 "$linux_arm64"
    end
    on_intel do
      url "$base/$tag/sm_${tag}_linux_amd64.tar.gz"
      sha256 "$linux_amd64"
    end
  end

  def install
    bin.install "sm"
  end

  test do
    assert_match "$ver", shell_output("#{bin}/sm --version")
  end
end
""")

content = TEMPLATE.substitute(
    ver=ver,
    tag=tag,
    base=BASE,
    darwin_arm64=sha("darwin_arm64.tar.gz"),
    darwin_amd64=sha("darwin_amd64.tar.gz"),
    linux_arm64=sha("linux_arm64.tar.gz"),
    linux_amd64=sha("linux_amd64.tar.gz"),
)

pathlib.Path("Formula").mkdir(exist_ok=True)
pathlib.Path("Formula/sm.rb").write_text(content)
print(f"✓ Formula/sm.rb 已同步到 {tag}")
