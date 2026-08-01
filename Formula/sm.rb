# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "0.3.0"

  on_macos do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.3.0/sm_v0.3.0_darwin_arm64.tar.gz"
      sha256 "0e22f3c5ec75a39a686134d304db5468947c67b4ae899a334166c1e02d1541b1"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.3.0/sm_v0.3.0_darwin_amd64.tar.gz"
      sha256 "162212ef8e1286b0aae33381ec413a135a56da213c93eb25c510a346d6cdc737"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.3.0/sm_v0.3.0_linux_arm64.tar.gz"
      sha256 "99de6a406e247d0a067c739e17edec0a49701d896d6ddef228a4b6400834446f"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.3.0/sm_v0.3.0_linux_amd64.tar.gz"
      sha256 "bbb918ee56b1485370337b6560630e833606addff55886e2c6481642908beb93"
    end
  end

  def install
    bin.install "sm"
  end

  test do
    assert_match "0.3.0", shell_output("#{bin}/sm --version")
  end
end
