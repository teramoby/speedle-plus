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
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/speedle-ads_darwin_arm64"
      sha256 "dfccd4696cf96c22c47f024583e72c32ccb984568fdf720dc46ab9dbebbef286"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/speedle-ads_darwin_amd64"
      sha256 "958f92bb1d97ebcd5cc875ccb9c306ad3d061b46ceda81b84068a50ce262be90"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/speedle-ads_linux_arm64"
      sha256 "01a8c2df8e4ea0a07be4a02fbe73496e2654b1b6333ce3e00b41f2b0e7f3b06e"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.1/speedle-ads_linux_amd64"
      sha256 "53f35405b1e6bb8256df0b4511cbb6cf26e519b7ed290a12b1a8846fdd13ab3b"
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
