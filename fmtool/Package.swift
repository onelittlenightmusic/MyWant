// swift-tools-version: 6.2
import PackageDescription

let package = Package(
    name: "fmtool",
    platforms: [.macOS(.v26)],
    targets: [
        .executableTarget(
            name: "fmtool",
            path: "Sources/fmtool"
        )
    ]
)
