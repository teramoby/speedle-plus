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
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/speedle-ads_darwin_arm64"
      sha256 "ea7a21ac34e57fff3d8d33a82d6bd385b7d192618901a85c3c3979683b2f73c3"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/speedle-ads_darwin_amd64"
      sha256 "ec643bbec75dc04c008825a81bf15ed8fdee9510143ae884e0cda1e40686e9f6"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/speedle-ads_linux_arm64"
      sha256 "10ec840a046937bc4baeef8b6e6c518e1b87f10d47a25b5d2db58e1ba4720cae"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/speedle-ads_linux_amd64"
      sha256 "5e77b552d542f36152357f2fb12145be885be8793261b528564214058f48156a"
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
