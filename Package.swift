// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "MonitorBar",
    platforms: [.macOS(.v14)],
    products: [.executable(name: "MonitorBar", targets: ["MonitorBar"])],
    targets: [
        .executableTarget(name: "MonitorBar", linkerSettings: [.linkedFramework("IOKit")])
    ]
)
