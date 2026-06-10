import Flutter
import UIKit

@main
@objc class AppDelegate: FlutterAppDelegate, FlutterImplicitEngineDelegate {
  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }

  func didInitializeImplicitFlutterEngine(_ engineBridge: FlutterImplicitEngineBridge) {
    GeneratedPluginRegistrant.register(with: engineBridge.pluginRegistry)

    // Platform channels mirroring Android's MainActivity: vpn control,
    // native log stream, native HTTP fetch.
    guard let registrar = engineBridge.pluginRegistry.registrar(
        forPlugin: "ClashForgeNativeChannels") else {
      return
    }
    let messenger = registrar.messenger()
    VpnChannelHandler.register(with: messenger)
    LogEventBridge.register(with: messenger)
    HttpChannelHandler.register(with: messenger)
  }
}
