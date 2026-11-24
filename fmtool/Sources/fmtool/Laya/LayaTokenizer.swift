// Copied from mywant-ios Common/LayaTokenizer.swift (44da2f3): the same Laya the apps decide
// with, so a decision is the same whichever worker takes it. Change both.

import Foundation

/// Laya's tokenizer, read from the `tokenizer.json` that ships beside the model
/// (Gemma-style BPE: spaces become ▁, a word is each run that starts with ▁,
/// pieces merge by rank, a character the vocabulary lacks falls back to its
/// UTF-8 bytes as <0xNN> tokens).
///
/// Only encoding, and only what Laya's sequences need: no special tokens are
/// matched in the text (the sequence builder puts CLS, SEP and MASK in by id),
/// no truncation beyond what the caller slices. One file for the iPhone and
/// the Mac: the same vocabulary, the same ids.
public final class LayaTokenizer: @unchecked Sendable {
    private let vocab: [String: Int32]
    /// A merge of two ids: its rank (lower merges first) and the id it makes.
    private let merges: [UInt64: (rank: Int32, merged: Int32)]
    private let byteIDs: [Int32] // <0x00> … <0xFF>
    private let unknownID: Int32
    /// Added tokens (newline and tab runs, <mask>, <eos>…), matched in the raw
    /// text before anything else, longest first; the text between them is
    /// what is normalised and split into words.
    private let added: [(text: String, id: Int32)]
    private let addedFirst: Set<Character>
    private var cache: [Substring: [Int32]] = [:]
    private let lock = NSLock()

    public enum Failure: Error { case unreadable(String) }

    public init(contentsOf url: URL) throws {
        let data = try Data(contentsOf: url)
        guard let root = try JSONSerialization.jsonObject(with: data) as? [String: Any],
              let model = root["model"] as? [String: Any],
              let rawVocab = model["vocab"] as? [String: Int],
              let rawMerges = model["merges"] as? [Any] else {
            throw Failure.unreadable("tokenizer.json has no BPE model")
        }
        var vocab = [String: Int32](minimumCapacity: rawVocab.count)
        for (piece, id) in rawVocab { vocab[piece] = Int32(id) }
        var merges = [UInt64: (rank: Int32, merged: Int32)](minimumCapacity: rawMerges.count)
        for (rank, m) in rawMerges.enumerated() {
            // ["a", "b"] in newer files, "a b" in older ones.
            let pair: [String]
            if let p = m as? [String] { pair = p } else if let s = m as? String {
                pair = s.split(separator: " ", maxSplits: 1, omittingEmptySubsequences: false).map(String.init)
            } else { continue }
            guard pair.count == 2, let a = vocab[pair[0]], let b = vocab[pair[1]],
                  let ab = vocab[pair[0] + pair[1]] else { continue }
            let key = Self.key(a, b)
            if merges[key] == nil { merges[key] = (Int32(rank), ab) }
        }
        self.vocab = vocab
        self.merges = merges
        self.byteIDs = (0..<256).map { vocab[String(format: "<0x%02X>", $0)] ?? 0 }
        self.unknownID = vocab[(model["unk_token"] as? String) ?? "<unk>"] ?? 3
        let addedTokens = (root["added_tokens"] as? [[String: Any]] ?? []).compactMap { t -> (String, Int32)? in
            guard let c = t["content"] as? String, !c.isEmpty, let id = t["id"] as? Int else { return nil }
            return (c, Int32(id))
        }
        self.added = addedTokens.sorted { $0.0.count > $1.0.count }
        self.addedFirst = Set(addedTokens.compactMap { $0.0.first })
    }

    private static func key(_ a: Int32, _ b: Int32) -> UInt64 {
        UInt64(UInt32(bitPattern: a)) << 32 | UInt64(UInt32(bitPattern: b))
    }

    /// The ids of `text`, without special tokens added around it
    /// (add_special_tokens=False) — though an added token written in the text
    /// is that token, as the original reads it.
    public func encode(_ text: String) -> [Int32] {
        var ids: [Int32] = []
        var segment = ""
        var i = text.startIndex
        scan: while i < text.endIndex {
            if addedFirst.contains(text[i]) {
                for (token, id) in added where text[i...].hasPrefix(token) {
                    ids += encodeSegment(segment)
                    segment = ""
                    ids.append(id)
                    i = text.index(i, offsetBy: token.count)
                    continue scan
                }
            }
            segment.append(text[i])
            i = text.index(after: i)
        }
        return ids + encodeSegment(segment)
    }

    /// Text with no added token in it: " " → "▁", a ▁ in front unless there is
    /// one (Metaspace, prepend always), then a word before each ▁.
    private func encodeSegment(_ text: String) -> [Int32] {
        guard !text.isEmpty else { return [] }
        var normalized = text.replacingOccurrences(of: " ", with: "▁")
        if !normalized.hasPrefix("▁") { normalized = "▁" + normalized }
        var ids: [Int32] = []
        var start = normalized.startIndex
        var i = normalized.index(after: start)
        while i < normalized.endIndex {
            if normalized[i] == "▁" {
                ids += word(normalized[start..<i])
                start = i
            }
            i = normalized.index(after: i)
        }
        ids += word(normalized[start..<normalized.endIndex])
        return ids
    }

    /// One word through BPE, cached.
    private func word(_ w: Substring) -> [Int32] {
        lock.lock()
        if let hit = cache[w] { lock.unlock(); return hit }
        lock.unlock()
        if let whole = vocab[String(w)] { return remember(w, [whole]) }
        // Characters (Unicode scalars, as the tokenizer's chars are), each its id
        // or its bytes.
        var symbols: [Int32] = []
        for scalar in w.unicodeScalars {
            if let id = vocab[String(scalar)] {
                symbols.append(id)
            } else {
                for byte in String(scalar).utf8 { symbols.append(byteIDs[Int(byte)]) }
            }
        }
        // Merge the lowest-ranked adjacent pair until none merges.
        while symbols.count > 1 {
            var best = -1
            var bestRank = Int32.max
            var bestMerged: Int32 = 0
            for j in 0..<(symbols.count - 1) {
                if let m = merges[Self.key(symbols[j], symbols[j + 1])], m.rank < bestRank {
                    best = j; bestRank = m.rank; bestMerged = m.merged
                }
            }
            if best < 0 { break }
            symbols.replaceSubrange(best...(best + 1), with: [bestMerged])
        }
        return remember(w, symbols)
    }

    private func remember(_ w: Substring, _ ids: [Int32]) -> [Int32] {
        lock.lock()
        if cache.count > 20_000 { cache.removeAll(keepingCapacity: true) }
        cache[w] = ids
        lock.unlock()
        return ids
    }
}
