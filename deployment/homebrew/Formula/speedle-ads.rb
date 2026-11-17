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
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/speedle-ads_darwin_arm64"
      sha256 "42bdccefb19f53c03e80cb99117695919788b1c0492c7d4661b43220447187ff"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/speedle-ads_darwin_amd64"
      sha256 "04e5c3e008d6c4277a566b1d09e22f6fb92aa118c272722b8a0cc5558c097ed7"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/speedle-ads_linux_arm64"
      sha256 "f236049ab87825c6d423f5eddba0d9fa8aeb3aa7447ea084f39bea556a9e6bc2"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.9.0/speedle-ads_linux_amd64"
      sha256 "1dc911fe204e2961c80918951ffaa35a7fd6c39805198093a22565754ed2d18b"
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
