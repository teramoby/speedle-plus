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
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/spctl_darwin_arm64"
      sha256 "f3f70816cfaf471bbbc436e4a256ae483c0a45e256f0fa1517af4cb368ef4013"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/spctl_darwin_amd64"
      sha256 "042f1e62d58b8f070ba33d6c55e97af4e1ca53b90439273f4a0d0ad3a1e06f8a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/spctl_linux_arm64"
      sha256 "a77e8e6b00bc9ebd1b72427013d7a642390d06048c7f36f1942f1e9661a22e92"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/spctl_linux_amd64"
      sha256 "fa97538952d881f924fb361b3cf442355ff94e377e1131dd2b80c897bd4d9100"
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
