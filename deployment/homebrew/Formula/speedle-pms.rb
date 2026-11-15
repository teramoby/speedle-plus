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
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/speedle-pms_darwin_arm64"
      sha256 "1f9832f8d44c3fd367adbbea62d590a6d0e627d273ba81d8fff83e36e5ac8a58"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/speedle-pms_darwin_amd64"
      sha256 "1fc0edc524a06f203b785a8d8f8617676662afbd91f9dff21721f488146ee8a7"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/speedle-pms_linux_arm64"
      sha256 "e103563ed2bcb93f8ad58cc1d9a8b67b598dea941e79a21a54459f451217757d"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/speedle-pms_linux_amd64"
      sha256 "3ee64f5e0207117fd0514863914940e536f4161930b732cccc664629a4801eda"
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
