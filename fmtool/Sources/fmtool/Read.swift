import Foundation
import ImageIO

// MARK: - Reading a photo, before anyone is asked about it
//
// `fmtool --read <image>` prints what can be known about a photo without a
// language model:
//
//   {"lines":["SCORE CARD | Nature Kochi黒潮 CC | 2026-9-22 14:57:36", …],
//    "meta":{"taken_at":"2026-09-22 14:57","camera":"Apple iPhone 16","size":"3024x4032"}}
//
// The words in it (OCR.swift's recogniser, exactly as --ocr gives them) and
// what the file says about itself — when it was taken, with what, where
// (ImageIO's EXIF, TIFF and GPS dictionaries). Each is read by what is good at
// it, once, on this machine; the model is then shown the photo with all of it
// (Picture.swift), and decides what answers the question rather than copying
// small print or guessing a date.
//
// A photo that has been through a share link has usually lost its EXIF on the
// way; then the meta is only its size, and the words are what is left to go on.

struct ReadResult: Encodable {
    let lines: [String]
    let meta: [String: String]
}

/// What the file says about itself: taken_at, camera, place, size.
func readImageMeta(at url: URL) -> [String: String] {
    guard let src = CGImageSourceCreateWithURL(url as CFURL, nil),
          let props = CGImageSourceCopyPropertiesAtIndex(src, 0, nil) as? [CFString: Any] else { return [:] }
    var meta: [String: String] = [:]
    if let w = props[kCGImagePropertyPixelWidth] as? Int, let h = props[kCGImagePropertyPixelHeight] as? Int {
        meta["size"] = "\(w)x\(h)"
    }
    let exif = props[kCGImagePropertyExifDictionary] as? [CFString: Any]
    let tiff = props[kCGImagePropertyTIFFDictionary] as? [CFString: Any]
    // "2026:09:22 14:57:36" → "2026-09-22 14:57"
    if let raw = (exif?[kCGImagePropertyExifDateTimeOriginal] ?? tiff?[kCGImagePropertyTIFFDateTime]) as? String {
        let parts = raw.split(separator: " ")
        if parts.count == 2 {
            meta["taken_at"] = parts[0].replacingOccurrences(of: ":", with: "-") + " " + parts[1].prefix(5)
        } else {
            meta["taken_at"] = raw
        }
    }
    let make = (tiff?[kCGImagePropertyTIFFMake] as? String)?.trimmingCharacters(in: .whitespaces) ?? ""
    let model = (tiff?[kCGImagePropertyTIFFModel] as? String)?.trimmingCharacters(in: .whitespaces) ?? ""
    let camera = model.hasPrefix(make) ? model : [make, model].filter { !$0.isEmpty }.joined(separator: " ")
    if !camera.isEmpty { meta["camera"] = camera }
    if let gps = props[kCGImagePropertyGPSDictionary] as? [CFString: Any],
       let lat = gps[kCGImagePropertyGPSLatitude] as? Double, let lon = gps[kCGImagePropertyGPSLongitude] as? Double {
        let ns = (gps[kCGImagePropertyGPSLatitudeRef] as? String) == "S" ? -1.0 : 1.0
        let ew = (gps[kCGImagePropertyGPSLongitudeRef] as? String) == "W" ? -1.0 : 1.0
        meta["place"] = String(format: "%.5f,%.5f", lat * ns, lon * ew)
    }
    return meta
}

/// Runs `--read` and exits: prints the JSON on stdout, or the reason on stderr.
func runRead(path: String) async -> Never {
    let url = URL(fileURLWithPath: path)
    do {
        let lines = try await recognizeLines(in: url)
        let data = try JSONEncoder().encode(ReadResult(lines: lines, meta: readImageMeta(at: url)))
        print(String(decoding: data, as: UTF8.self))
        exit(0)
    } catch {
        printErr("read failed: \(error.localizedDescription)")
        exit(1)
    }
}
