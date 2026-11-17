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
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/spctl_darwin_arm64"
      sha256 "f142af6768105e1d0c571aa272baf9a87ba5673df11c82dbc0bc5e3698c7d332"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/spctl_darwin_amd64"
      sha256 "5c6846695842f832fe01f58dac253532b4107f8915ac801c77b123f3b42e318c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/spctl_linux_arm64"
      sha256 "f78b6a2b4700a362fc50d520649cdb40ca00ec50969658815e25487fc1491b83"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/spctl_linux_amd64"
      sha256 "23377d10b03c7bfef095631487e95f9c43ad66d7b24f9e2eeb372e0549cb1305"
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
