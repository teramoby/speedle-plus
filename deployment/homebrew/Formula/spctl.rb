# typed: false
# frozen_string_literal: true

class Spctl < Formula
  desc "Command-line tool for Speedle authorization policies"
  homepage "https://engine.speedle.io"
  license "UPL-1.0"

  livecheck do
    url :stable
    strategy :github_latest
  end

  on_macos do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/spctl_darwin_arm64"
      sha256 "9f162a9eef2f3b02bb05b3d75b405db56d73b3756010d25da8ac827b6857c453"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/spctl_darwin_amd64"
      sha256 "1e4932895abb55d49d677089223e823a0895b0ed6ab294cecd62d6a7c1cd6d46"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/spctl_linux_arm64"
      sha256 "713ac1cd2fc4bc48f3b2ebec1fc7f1d08112dae8851abc476c9e5e743e663a9d"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/spctl_linux_amd64"
      sha256 "362b6859494c197bd90b2c7f1660e61f609330b2737bcd2769f9192735080bb4"
    end
  end

  def install
    binaries = Dir["#{name}_*"]
    odie "Expected one #{name} release binary, found #{binaries.inspect}" if binaries.length != 1

    # macOS already has /usr/sbin/spctl (Apple Gatekeeper). Linux also gets spctl.
    bin.install binaries.fetch(0) => "speedle-spctl"
    bin.install_symlink bin/"speedle-spctl" => "spctl" if OS.linux?
  end

  def caveats
    <<~EOS
      On macOS this formula installs the speedle-spctl command and does not
      install a command named spctl. /usr/sbin/spctl is Apple Gatekeeper.
      On Linux the same program is also available as spctl.
    EOS
  end

  test do
    assert_match "Usage:", shell_output("#{bin}/speedle-spctl --help")
    if OS.linux?
      assert_match "Usage:", shell_output("#{bin}/spctl --help")
    else
      refute_path_exists bin/"spctl"
    end
  end
end
