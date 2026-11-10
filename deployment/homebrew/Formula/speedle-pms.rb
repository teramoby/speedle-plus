# typed: false
# frozen_string_literal: true

class SpeedlePms < Formula
  desc "Policy management service for Speedle"
  homepage "https://engine.speedle.io"
  license "UPL-1.0"

  livecheck do
    url :stable
    strategy :github_latest
  end

  on_macos do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/speedle-pms_darwin_arm64"
      sha256 "cc6f07ee9c3af27422f0ca1d430d9f16fc641bc8330e19260112d47e24a5ec56"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/speedle-pms_darwin_amd64"
      sha256 "83cd1de70a47b64f646962b40b4db0fde2a563a7c7de9a9365c2cba0ccf99351"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/speedle-pms_linux_arm64"
      sha256 "11a0ebcf2f7fb53716ca46eece7a7f8a235e098faeae4ded8294aaa3591b61fc"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/speedle-pms_linux_amd64"
      sha256 "45d57502a2071044b271224d1566bc0d7835684273e0dbf8ab46e1a1c158202a"
    end
  end

  def install
    binaries = Dir["#{name}_*"]
    odie "Expected one #{name} release binary, found #{binaries.inspect}" if binaries.length != 1

    bin.install binaries.fetch(0) => name
    # launchd and systemd refuse a missing WorkingDirectory. Create the shared
    # store directory here so a fresh prefix can start the service.
    (var/"speedle").mkpath
    (var/"log").mkpath
  end

  service do
    run [
      opt_bin/"speedle-pms",
      "--store-type",
      "file",
      "--filestore-loc",
      var/"speedle/policies.json",
      "--endpoint",
      "127.0.0.1:6733",
      "--insecure=true",
    ]
    keep_alive true
    working_dir var/"speedle"
    log_path var/"log/speedle-pms.log"
    error_log_path var/"log/speedle-pms.log"
  end

  test do
    assert_path_exists var/"speedle"
    assert_path_exists var/"log"
    system bin/"speedle-pms", "--help"
  end
end
