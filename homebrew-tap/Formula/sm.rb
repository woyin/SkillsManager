class Sm < Formula
  desc "CLI tool for managing AI agent skills and MCP configurations"
  homepage "https://github.com/woyin/skills-manager"
  url "https://github.com/woyin/skills-manager/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PLACEHOLDER_SHA256"
  license "MIT"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "-o", bin/"sm", "."
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sm --version")
    assert_match "SkillsManager", shell_output("#{bin}/sm --help")
  end
end
