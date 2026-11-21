# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "0.4.0"

  on_macos do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.4.0/sm_v0.4.0_darwin_arm64.tar.gz"
      sha256 "56f7973788dabc76b568651ddb14e5c88aadb59f7b4e8eb30a07998d31f2a9da"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.4.0/sm_v0.4.0_darwin_amd64.tar.gz"
      sha256 "7c7438f37e633ae1052d1e1e03d3a5f8b5a10d7f88e57cc2495063e287c7e34e"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.4.0/sm_v0.4.0_linux_arm64.tar.gz"
      sha256 "11010ac02e56897471dd7264f48c3978c7ff29a4e104bfcc845827a3c0452e09"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.4.0/sm_v0.4.0_linux_amd64.tar.gz"
      sha256 "e42f683b0abf4111930a365c6626e2ca63c9365abb28cc3667596a6c00e15936"
    end
  end

  def install
    bin.install "sm"
    generate_completions_from_executable(bin/"sm", "completion")
  end

  test do
    assert_match "0.4.0", shell_output("#{bin}/sm --version")
  end
end
