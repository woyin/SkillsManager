# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "0.2.0"

  on_macos do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.0/sm_v0.2.0_darwin_arm64.tar.gz"
      sha256 "66e58c4ef8a6d636c6ee343ba5a26321bd6cf70b593361e4176759fcdbebabf1"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.0/sm_v0.2.0_darwin_amd64.tar.gz"
      sha256 "b2b7e3bf23674001a0dfd7a0e86f970be57ec03dc7b9d68f6fe4b97b0f78c83e"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.0/sm_v0.2.0_linux_arm64.tar.gz"
      sha256 "e7e4fc4948ec511cc2d82ddeba7c816b78473b235b5743437ba146b2a084aa8f"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.0/sm_v0.2.0_linux_amd64.tar.gz"
      sha256 "78b91ed5440a40a7aacb4c0bdb856c9a7f8120f515ec490da4be0e77e5daeeec"
    end
  end

  def install
    bin.install "sm"
  end

  test do
    assert_match "0.2.0", shell_output("#{bin}/sm --version")
  end
end
