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
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/speedle-pms_darwin_arm64"
      sha256 "d9d8d1130a493fcec71ff1b2674301ff8cf3df29f2946a088154cf26ba958ffd"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/speedle-pms_darwin_amd64"
      sha256 "30e50f63e8459513db45785e09d0d785b64f11efbb63c9d0e5bc29413efae279"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/speedle-pms_linux_arm64"
      sha256 "14f6465a02474a63901cfe82ed3b091ed3f01ed1004e408475f1645f5a571132"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/speedle-pms_linux_amd64"
      sha256 "ed77e9f43f189159b9686f25dd2d9856c5c27d047e428dac635e2950f50608ed"
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
