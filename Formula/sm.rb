# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "0.2.3"

  on_macos do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.3/sm_v0.2.3_darwin_arm64.tar.gz"
      sha256 "ba73adc15bd2b71f4489c0199596869432f7730c5b6f86d97ff1881a5a09bea9"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.3/sm_v0.2.3_darwin_amd64.tar.gz"
      sha256 "4e4f5b98d6d95240eb63e80c116bcef9c0060b6d227bcad507eadc3fc5b7bb43"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.3/sm_v0.2.3_linux_arm64.tar.gz"
      sha256 "b8cdc6c5fff94d6cb3080be5c48fcbc3f4f8ce729ced0cf4b55038e54fa0df9a"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.3/sm_v0.2.3_linux_amd64.tar.gz"
      sha256 "12e8e142d1f4f5304e247a69897396a1d62072554dbf429cf416312f5c8e6b50"
    end
  end

  def install
    bin.install "sm"
  end

  test do
    assert_match "0.2.3", shell_output("#{bin}/sm --version")
  end
end
