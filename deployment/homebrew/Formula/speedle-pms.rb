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
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/speedle-pms_darwin_arm64"
      sha256 "32b6da22a11fa3ec39b65498ca73829e51c4ff53af9e1be4a098a01bb9887cbf"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/speedle-pms_darwin_amd64"
      sha256 "e815428f26345a5da38db0bd4c7bdd445b354eacea7a2bd768c7f4f426c21a08"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/speedle-pms_linux_arm64"
      sha256 "4ab8caab9f6b6b939e8010ef7b66c8f28ec67b9007cbfec2e351d04108533dd3"
    end
    on_intel do
      url "https://github.com/teramoby/speedle-plus/releases/download/v1.7.0/speedle-pms_linux_amd64"
      sha256 "0abc94360c4ad63515c85e4b14acf1aa98dce0ff518e711c991aee94e76dccb9"
    end
  end

  def install
    binaries = Dir["#{name}_*"]
    odie "Expected one #{name} release binary, found #{binaries.inspect}" if binaries.length != 1

    bin.install binaries.fetch(0) => name
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
    system bin/"speedle-pms", "--help"
  end
end
