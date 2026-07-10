# clashforge_mobile

A new Flutter project.

## Getting Started

This project is a starting point for a Flutter application.

A few resources to get you started if this is your first Flutter project:

- [Learn Flutter](https://docs.flutter.dev/get-started/learn-flutter)
- [Write your first Flutter app](https://docs.flutter.dev/get-started/codelab)
- [Flutter learning resources](https://docs.flutter.dev/reference/learning-resources)

For help getting started with Flutter development, view the
[online documentation](https://docs.flutter.dev/), which offers tutorials,
samples, guidance on mobile development, and a full API reference.

## Android VPN power profiles

Settings exposes three runtime profiles:

- **Battery Saver** uses the lite rule template when the subscription does not
  provide its own rules, disables connection racing, lengthens automatic
  latency checks, and disables sniffing in fake-IP mode.
- **Balanced** is the default. It retains compatibility-oriented sniffing and
  TCP racing, but reduces log and latency-test frequency.
- **Performance** restores the shortest latency-test interval and unified
  delay measurement.

App Routing can further reduce packet-processing cost by limiting the VPN to
selected Android apps or bypassing selected high-traffic apps.

## Measuring Android power use

Connect one Android device, start the desired VPN/profile, keep the test
conditions stable, and run from the repository root:

```powershell
./mobile/scripts/measure-android-power.ps1 -Minutes 30 -Scenario idle
```

Reports are written under `build/power-reports`. Compare at least three runs
per profile on the same device and network. Use `-Scenario auto` when the Auto
proxy group is selected and `-Scenario download` for a controlled transfer.
