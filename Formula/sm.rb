# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "0.2.6"

  on_macos do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.6/sm_v0.2.6_darwin_arm64.tar.gz"
      sha256 "f2a3cabb309fc5ed212aae106aa3dfdaa68c0046ddedf449c389b8e7b9078f52"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.6/sm_v0.2.6_darwin_amd64.tar.gz"
      sha256 "d614cd6962992f6b4ba7428c2378773c86d05b4e953dd274c1000643ad3de484"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.6/sm_v0.2.6_linux_arm64.tar.gz"
      sha256 "2da928e15fb102e0f0e10594fc9ebc03c1cbcff42f26d5a3bdd7915420296062"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.2.6/sm_v0.2.6_linux_amd64.tar.gz"
      sha256 "119d59a88611762d1803758be9419a8239f3960a39b5c4d106429a28afaaf981"
    end
  end

  def install
    bin.install "sm"
  end

  test do
    assert_match "0.2.6", shell_output("#{bin}/sm --version")
  end
end
