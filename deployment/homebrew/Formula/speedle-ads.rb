# typed: false
# frozen_string_literal: true

class SpeedleAds < Formula
  desc "Authorization decision service for Speedle"
  homepage "https://engine.speedle.io"
  license "UPL-1.0"

  livecheck do
    url :stable
    strategy :github_latest
  end

  on_macos do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/speedle-ads_darwin_arm64"
      sha256 "32f7bffac9682a3d2eca57e080d227fb97e28714279d9b29cb888b8d34b2f086"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/speedle-ads_darwin_amd64"
      sha256 "c1955f6e68d01de469dda3a6d90f645df8e752aa125529cb1beff13fd29b028c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/speedle-ads_linux_arm64"
      sha256 "d5de39d152feafc4be1d21c880410f5127f26756cbfe1157724a510dd223dad8"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.8.0/speedle-ads_linux_amd64"
      sha256 "e97ac426920ef977ec331e1c87ef3fbfdf1a6413baf726aa9df39b2ca7ffc015"
    end
  end

  def install
    binaries = Dir["#{name}_*"]
    odie "Expected one #{name} release binary, found #{binaries.inspect}" if binaries.length != 1

    bin.install binaries.fetch(0) => name
    # The same directory PMS uses. A fresh prefix does not have it until install.
    (var/"speedle").mkpath
    (var/"log").mkpath
  end

  service do
    run [
      opt_bin/"speedle-ads",
      "--store-type",
      "file",
      "--filestore-loc",
      var/"speedle/policies.json",
      "--endpoint",
      "127.0.0.1:6734",
      "--insecure=true",
    ]
    keep_alive true
    working_dir var/"speedle"
    log_path var/"log/speedle-ads.log"
    error_log_path var/"log/speedle-ads.log"
  end

  test do
    assert_path_exists var/"speedle"
    assert_path_exists var/"log"
    system bin/"speedle-ads", "--help"
  end
end
