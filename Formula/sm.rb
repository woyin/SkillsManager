# Homebrew formula for sm (SkillsManager).
#
# 此文件由 .github/workflows/release.yml 在每次发布时自动重新生成
# (version、各 url、各 sha256 会被覆盖)。请勿手动修改这些字段;
# 如需调整 install/test 逻辑,改 .github/scripts/sync_formula.py 中的模板。
class Sm < Formula
  desc "SkillsManager — manage AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/SkillsManager"
  license "MIT"
  version "0.4.1"

  on_macos do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.4.1/sm_v0.4.1_darwin_arm64.tar.gz"
      sha256 "87775560c0acd9071991dbcb1bdcb16293cf7ba62456d10c725d17cfe516a735"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.4.1/sm_v0.4.1_darwin_amd64.tar.gz"
      sha256 "6dc10923014a9a34c7f9ae99b9beaf68ce51585749fd69c40a510a7a1b72f09f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.4.1/sm_v0.4.1_linux_arm64.tar.gz"
      sha256 "14455acfc5dd0af6f1b766e34dc9a2442a70460d8c249910cdbc037ae84724af"
    end
    on_intel do
      url "https://github.com/woyin/SkillsManager/releases/download/v0.4.1/sm_v0.4.1_linux_amd64.tar.gz"
      sha256 "e836e4867add96082f75e6a170f7f101493424dc6a1f4261e058f59f40261be1"
    end
  end

  def install
    bin.install "sm"
    generate_completions_from_executable(bin/"sm", "completion")
  end

  test do
    assert_match "0.4.1", shell_output("#{bin}/sm --version")
  end
end
