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
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/spctl_darwin_arm64"
      sha256 "b47f45a3d9d85417eb0a605849f22c0a0161e8b6e711fd5e2653f4df7783d50c"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/spctl_darwin_amd64"
      sha256 "a8a69bec684918aecb35d60c75c25c6cffee3cd28afce01ef2fd33e29e78168c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/spctl_linux_arm64"
      sha256 "f645c33bac7b096d18085cb44354c3bc9100aa4891a3f6fce64828a4a6dc5f10"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/spctl_linux_amd64"
      sha256 "6e773d1cd519ca64698ad99d7b3a8e7bd19ad1465d81b3e065fbbd51a6e20a75"
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
