import AppKit
import ImagePlayground

/// A picture drawn on this Mac for a page in Chrome (NativeHost's `draw`): the
/// picture under something on the board, 作る in the GUI's ArtMaker.
///
/// Only Apple's Image Playground screen draws here. ImageCreator, drawing with
/// no screen, is refused (notSupported on macOS 27, as on the iPhone from iOS
/// 27), so the screen is shown — in a window of this process's own, which a
/// command with no app bundle can do — with what to draw already in it, and
/// the picture the person keeps there (Done) is the one handed back. Cancel, or
/// the window closed, hands back none.
///
/// Handed back as the iPhone hands its drawing back: a PNG, 384 on a side — a
/// ball's picture is small — as a data URL.
@MainActor
enum Drawing {
    static let side: CGFloat = 384

    static var available: Bool { ImagePlaygroundViewController.isAvailable }

    static func draw(prompt: String) async -> [String: Any] {
        let prompt = prompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !prompt.isEmpty else { return ["error": "何を描くかがありません"] }
        guard available else { return ["error": "この Mac では Image Playground が使えません"] }
        let url: URL? = await withCheckedContinuation { done in
            Sheet(prompt: prompt, done: { done.resume(returning: $0) }).show()
        }
        guard let url else { return ["cancelled": true] }
        defer { try? FileManager.default.removeItem(at: url) }
        guard let png = png(url) else { return ["error": "描いた絵を読めませんでした"] }
        return ["image": "data:image/png;base64," + png.base64EncodedString(), "prompt": prompt]
    }

    /// The picture at url (Image Playground's HEIC), as a PNG `side` square.
    private static func png(_ url: URL) -> Data? {
        guard let image = NSImage(contentsOf: url),
              let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: Int(side), pixelsHigh: Int(side),
                                         bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
                                         colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0) else { return nil }
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
        image.draw(in: NSRect(x: 0, y: 0, width: side, height: side))
        NSGraphicsContext.restoreGraphicsState()
        return rep.representation(using: .png, properties: [:])
    }

    /// The window, and what is done when it is: one answer, however it ends.
    @MainActor private final class Sheet: NSObject, ImagePlaygroundViewController.Delegate, NSWindowDelegate {
        private let prompt: String
        private var done: ((URL?) -> Void)?
        private var window: NSWindow?
        /// Kept while the window is up: nothing else holds the delegate.
        private static var open: Set<Sheet> = []

        init(prompt: String, done: @escaping (URL?) -> Void) {
            self.prompt = prompt
            self.done = done
        }

        func show() {
            let controller = ImagePlaygroundViewController()
            controller.delegate = self
            controller.concepts = [.text(prompt)]
            let window = NSWindow(contentViewController: controller)
            window.title = "MyWant — 背景を作る"
            window.setContentSize(NSSize(width: 820, height: 620))
            window.isReleasedWhenClosed = false
            window.delegate = self
            window.center()
            self.window = window
            Self.open.insert(self)
            // In front of Chrome, where 作る was pressed: a process with no Dock
            // icon (accessory) has to be brought forward to be seen at all.
            NSApp.activate(ignoringOtherApps: true)
            window.makeKeyAndOrderFront(nil)
            window.orderFrontRegardless()
        }

        private func finish(_ url: URL?) {
            guard let done else { return }
            self.done = nil
            done(url)
            window?.delegate = nil
            window?.close()
            Self.open.remove(self)
        }

        func imagePlaygroundViewController(_ controller: ImagePlaygroundViewController, didCreateImageAt imageURL: URL) {
            finish(imageURL)
        }

        func imagePlaygroundViewControllerDidCancel(_ controller: ImagePlaygroundViewController) {
            finish(nil)
        }

        func windowWillClose(_ notification: Notification) {
            finish(nil)
        }
    }
}
