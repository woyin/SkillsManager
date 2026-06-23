# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "0.2.1"

  on_macos do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.1/sm_v0.2.1_darwin_arm64.tar.gz"
      sha256 "a91c87343ee7f2d31ef25d3c96fdeb38fabfbe8ee0d232cafe2cd30da39e8fd0"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.1/sm_v0.2.1_darwin_amd64.tar.gz"
      sha256 "e6475b5a5e8108e0148651d877c6feb1a7d609aa63a73dd4001f37ead82e560f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.1/sm_v0.2.1_linux_arm64.tar.gz"
      sha256 "687917f09032c8e6ab5187c2b98b155c9da5c4569f8aa17eea01a72900825446"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.1/sm_v0.2.1_linux_amd64.tar.gz"
      sha256 "0f8174971de81b1b306da22daa8e92031fc6386a654179a6f1e2a74ea3b0e777"
    end
  end

  def install
    bin.install "sm"
  end

  test do
    assert_match "0.2.1", shell_output("#{bin}/sm --version")
  end
end
