import Foundation
import Vision

// MARK: - Reading the text in a picture
//
// `fmtool --ocr <image>` prints what is written in an image and nothing else:
//
//   {"lines":["SCORE CARD | Nature Kochi黒潮 CC | 2026-9-22 14:57:36", …]}
//
// No language model is involved. The model is good at understanding what a
// picture is of and poor at copying what is written in it — asked to read a
// golf score card it got the total right and the date, the pars and most of
// the holes wrong — while the Vision text recogniser copies exactly what it
// can see and makes up nothing. So the words are read here, once, and whoever
// answers a question about the picture later is handed them.
//
// The recogniser's boxes are put back into reading order: grouped into lines by
// their height on the page, and each line read left to right with its cells
// separated by " | ". A table stays a table that way — a row of a score card is
// one line, its columns in order.

struct OCRResult: Encodable {
    let lines: [String]
}

func recognizeLines(in url: URL) async throws -> [String] {
    var request = RecognizeTextRequest()
    request.recognitionLanguages = [Locale.Language(identifier: "ja-JP"), Locale.Language(identifier: "en-US")]
    request.recognitionLevel = .accurate
    let observations = try await request.perform(on: url)

    // (text, left edge, vertical centre measured from the top)
    let boxes: [(text: String, x: CGFloat, y: CGFloat)] = observations.compactMap { o in
        guard let text = o.topCandidates(1).first?.string else { return nil }
        let b = o.boundingBox
        return (text, b.origin.x, 1 - (b.origin.y + b.height / 2))
    }.sorted { $0.y < $1.y }

    // Two boxes are on one line when their centres are within about a line's
    // height of each other; measured against the line's first box so a slightly
    // tilted photo does not chain every row of a table into one.
    let lineTolerance: CGFloat = 0.012
    var lines: [[(text: String, x: CGFloat, y: CGFloat)]] = []
    for box in boxes {
        if let first = lines.last?.first, abs(first.y - box.y) < lineTolerance {
            lines[lines.count - 1].append(box)
        } else {
            lines.append([box])
        }
    }
    return lines.map { $0.sorted { $0.x < $1.x }.map(\.text).joined(separator: " | ") }
}

/// Runs `--ocr` and exits: prints the JSON on stdout, or the reason on stderr.
func runOCR(path: String) async -> Never {
    let url = URL(fileURLWithPath: path)
    do {
        let lines = try await recognizeLines(in: url)
        let data = try JSONEncoder().encode(OCRResult(lines: lines))
        print(String(decoding: data, as: UTF8.self))
        exit(0)
    } catch {
        printErr("ocr failed: \(error.localizedDescription)")
        exit(1)
    }
}
