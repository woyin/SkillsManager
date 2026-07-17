# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "0.2.7"

  on_macos do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.7/sm_v0.2.7_darwin_arm64.tar.gz"
      sha256 "c9fc2259e44cc3021470f03138788779a18e07101521b2b2d5bf96db72779fad"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.7/sm_v0.2.7_darwin_amd64.tar.gz"
      sha256 "1fb09729d5f13d82efba118479c5f15a50981dd79b21604d53cc9b61bc503b3f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.7/sm_v0.2.7_linux_arm64.tar.gz"
      sha256 "77fc6a79e7cbe8729a45494be0df8daf1ff88e5087ee62d36df622cee1b0739b"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.7/sm_v0.2.7_linux_amd64.tar.gz"
      sha256 "769fcd64ce05d95b819f1cad8a63bafd5c8bf3b87684c0fc67df179eed3c36de"
    end
  end

  def install
    bin.install "sm"
  end

  test do
    assert_match "0.2.7", shell_output("#{bin}/sm --version")
  end
end
