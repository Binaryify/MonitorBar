#!/bin/bash
# Builds dist/MonitorBar.app (ad-hoc signed). Pass --install to copy it to ~/Applications.
set -euo pipefail
cd "$(dirname "$0")/.."

swift build -c release
APP="dist/MonitorBar.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
cp .build/release/MonitorBar "$APP/Contents/MacOS/MonitorBar"
cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>MonitorBar</string>
<key>CFBundleIdentifier</key><string>local.binaryify.MonitorBar</string>
<key>CFBundleName</key><string>MonitorBar</string>
<key>CFBundleDisplayName</key><string>MonitorBar</string>
<key>CFBundleVersion</key><string>1</string>
<key>CFBundleShortVersionString</key><string>1.0</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>14.0</string>
<key>LSUIElement</key><true/>
<key>NSHighResolutionCapable</key><true/>
<key>NSLocalNetworkUsageDescription</key><string>通过局域网切换显示器输入源、调节音量。</string>
<key>NSBonjourServices</key><array><string>_rc._tcp</string></array>
<key>NSAppTransportSecurity</key><dict><key>NSAllowsLocalNetworking</key><true/></dict>
</dict></plist>
PLIST
codesign --force --sign - --identifier local.binaryify.MonitorBar "$APP"
echo "Built $APP"

if [[ "${1:-}" == "--install" ]]; then
  mkdir -p ~/Applications
  rm -rf ~/Applications/MonitorBar.app
  cp -R "$APP" ~/Applications/
  echo "Installed ~/Applications/MonitorBar.app"
fi
