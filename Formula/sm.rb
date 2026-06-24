# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "0.2.2"

  on_macos do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.2/sm_v0.2.2_darwin_arm64.tar.gz"
      sha256 "18a688742961e2a2bc317f188711816ec1886b8828db74f43f8975e37676e7c0"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.2/sm_v0.2.2_darwin_amd64.tar.gz"
      sha256 "79f1b656c2a0387618d020a9a3a9124702138fd121cfc32f73ec18daf1441327"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.2/sm_v0.2.2_linux_arm64.tar.gz"
      sha256 "61218f4913c6f741e58413aa9303ce418f8900e468247a1324eed4e62d904969"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.2/sm_v0.2.2_linux_amd64.tar.gz"
      sha256 "2fbbcd7ab6741b23acfe2a91b8d6405f50135179f0e0786226d9783a0b890b12"
    end
  end

  def install
    bin.install "sm"
  end

  test do
    assert_match "0.2.2", shell_output("#{bin}/sm --version")
  end
end
