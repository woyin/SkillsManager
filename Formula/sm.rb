# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "0.2.5"

  on_macos do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.5/sm_v0.2.5_darwin_arm64.tar.gz"
      sha256 "695d989a62dfc3b0bc472ecf556411d9f3259d32a5ca1adfe823ac91f6b47a0a"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.5/sm_v0.2.5_darwin_amd64.tar.gz"
      sha256 "c827cf058967bc5e11c967a69e94f8a215f628cae136f07262d03116b1564d48"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.5/sm_v0.2.5_linux_arm64.tar.gz"
      sha256 "9f77bef61936b1cc37ac123259f2d702a59a920ed554d553d5b79c5b1dd5ea56"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.5/sm_v0.2.5_linux_amd64.tar.gz"
      sha256 "ae52d6d9c2812e1580b7d83fce6dde6b384085f33d3f2cfef0e6f90d464b14c8"
    end
  end

  def install
    bin.install "sm"
  end

  test do
    assert_match "0.2.5", shell_output("#{bin}/sm --version")
  end
end
