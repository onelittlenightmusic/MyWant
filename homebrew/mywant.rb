# This file is a template for the homebrew-mywant tap.
# Copy it to: https://github.com/onelittlenightmusic/homebrew-mywant/Formula/mywant.rb
# GoReleaser will keep it updated automatically after each release.
class Mywant < Formula
  desc "Declarative chain programming system - express what you want via YAML, autonomous agents execute it"
  homepage "https://github.com/onelittlenightmusic/MyWant"
  version "PLACEHOLDER"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/onelittlenightmusic/MyWant/releases/download/vPLACEHOLDER/mywant_vPLACEHOLDER_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_ARM64"
    else
      url "https://github.com/onelittlenightmusic/MyWant/releases/download/vPLACEHOLDER/mywant_vPLACEHOLDER_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_AMD64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/onelittlenightmusic/MyWant/releases/download/vPLACEHOLDER/mywant_vPLACEHOLDER_linux_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_ARM64"
    else
      url "https://github.com/onelittlenightmusic/MyWant/releases/download/vPLACEHOLDER/mywant_vPLACEHOLDER_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_AMD64"
    end
  end

  def install
    bin.install "mywant"
  end

  test do
    system "#{bin}/mywant", "--version"
  end
end
