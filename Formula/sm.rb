# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "0.2.4"

  on_macos do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.4/sm_v0.2.4_darwin_arm64.tar.gz"
      sha256 "a5d45651368cf63335dfab32d829c213e7201bb9c57341061086c20cd56f5deb"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.4/sm_v0.2.4_darwin_amd64.tar.gz"
      sha256 "7aada772be47ccd98c2428ac03730dbf9b343cd82b8738c4a3e02814b7685f79"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.4/sm_v0.2.4_linux_arm64.tar.gz"
      sha256 "79394ea7c1647488dd8f7c760b7ebee625926d7f16d434b49dcded9fa477cff7"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.4/sm_v0.2.4_linux_amd64.tar.gz"
      sha256 "b3a0178ec084d8306176830f90f24f5dbf426c1b5eec392fb82c8463d3efb3d5"
    end
  end

  def install
    bin.install "sm"
  end

  test do
    assert_match "0.2.4", shell_output("#{bin}/sm --version")
  end
end
