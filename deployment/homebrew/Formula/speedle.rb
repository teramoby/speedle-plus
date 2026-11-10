# typed: false
# frozen_string_literal: true

# Installs the command-line tool, PMS and ADS. Those formulas own the binaries.
class Speedle < Formula
  desc "Authorization services and command-line tool for Speedle"
  homepage "https://engine.speedle.io"
  url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/checksums.txt"
  sha256 "8260f98dcecb8094b82243df23b4bc30a555633cc3df064dfc6a37ac9a109d4a"
  license "UPL-1.0"

  livecheck do
    url :stable
    strategy :github_latest
  end

  depends_on "spctl"
  depends_on "speedle-ads"
  depends_on "speedle-pms"

  def install
    # A top-level README is a metafile, which Homebrew treats as an empty keg.
    (pkgshare/"README").write <<~EOS
      This formula installs speedle-spctl, speedle-pms and speedle-ads.
      On macOS the command is speedle-spctl. On Linux it is also available as spctl.
      Start the services with `brew services start speedle-pms` and `brew services start speedle-ads`.
      They listen on 127.0.0.1:6733 and 127.0.0.1:6734 and share #{var}/speedle/policies.json.
    EOS
  end

  def caveats
    <<~EOS
      The command-line tool is speedle-spctl. On Linux, spctl is the same program.
      Start the demo services with:
        brew services start speedle-pms
        brew services start speedle-ads
    EOS
  end

  test do
    assert_path_exists pkgshare/"README"
    assert_path_exists formula_opt_bin("spctl")/"speedle-spctl"
    assert_path_exists formula_opt_bin("speedle-pms")/"speedle-pms"
    assert_path_exists formula_opt_bin("speedle-ads")/"speedle-ads"
  end
end
