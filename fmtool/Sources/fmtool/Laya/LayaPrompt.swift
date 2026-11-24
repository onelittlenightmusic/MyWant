// Copied from mywant-ios Common/LayaPrompt.swift (44da2f3): the same Laya the apps decide
// with, so a decision is the same whichever worker takes it. Change both.

import Foundation

/// A typed question, as the server sends it (engine/server/robot_initiative.go,
/// decisionQuestion) and as Laya takes it: choice (criteria: label →
/// description), score (an ordered list of levels) or noul (yes/no).
public struct LayaQuestion: Codable, Sendable, Hashable {
    public var type: String
    public var instructions: String
    /// choice: label → description. score: "0", "1", … → level text (an array on
    /// the wire, see init(from:)). noul: optional "false"/"true" → description.
    public var criteria: [String: String]
    /// The labels in the order they are offered — the order of the JSON
    /// object's keys for a choice, as Laya reads a Python dict.
    public var labels: [String]

    public init(type: String, instructions: String, labels: [String], criteria: [String: String]) {
        self.type = type; self.instructions = instructions; self.labels = labels; self.criteria = criteria
    }

    enum CodingKeys: String, CodingKey { case type, instructions, criteria, labels }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        type = try c.decode(String.self, forKey: .type)
        instructions = try c.decodeIfPresent(String.self, forKey: .instructions) ?? ""
        if let list = try? c.decode([String].self, forKey: .criteria) {
            if type == "choice" {
                // Labels with no description.
                labels = list
                criteria = Dictionary(list.map { ($0, "") }, uniquingKeysWith: { a, _ in a })
            } else {
                labels = list.indices.map(String.init)
                criteria = Dictionary(uniqueKeysWithValues: zip(labels, list))
            }
        } else {
            criteria = (try? c.decode([String: String].self, forKey: .criteria)) ?? [:]
            // A JSON object has no order once decoded: the sender may say it
            // (labels); else sorted, which the server's own maps are.
            labels = (try? c.decode([String].self, forKey: .labels)) ?? criteria.keys.sorted()
        }
        if type == "noul" { labels = ["false", "true"] }
    }

    public func encode(to encoder: any Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(type, forKey: .type)
        try c.encode(instructions, forKey: .instructions)
        try c.encode(criteria, forKey: .criteria)
        try c.encode(labels, forKey: .labels)
    }

    /// The option texts Laya scores, in label order (laya's render_options).
    var optionTexts: [String] {
        switch type {
        case "score":
            return labels.enumerated().map { "level \($0.offset): \(criteria[$0.element] ?? "")" }
        case "noul":
            return [
                "false: " + (criteria["false"].flatMap { $0.isEmpty ? nil : $0 } ?? "no, the statement does not hold"),
                "true: " + (criteria["true"].flatMap { $0.isEmpty ? nil : $0 } ?? "yes, the statement holds"),
            ]
        default:
            return labels.map { label in
                let d = criteria[label] ?? ""
                return d.isEmpty ? label : "\(label): \(d)"
            }
        }
    }

    /// The qtype_onehot slot: choice, score, noul.
    var typeIndex: Int { type == "score" ? 1 : type == "noul" ? 2 : 0 }
}

/// One question about one state, laid out as Laya reads it (laya's
/// build_sequence): `[CLS] <type> question: <instructions> [SEP] [MASK] opt …
/// [MASK] opt [SEP] <state> [SEP]`, the state cut to what is left of the window.
public struct LayaSequence: Sendable {
    public let ids: [Int32]
    /// Where each option's [MASK] sits — its logit is read there.
    public let markers: [Int]

    public static let cls: Int32 = 2, sep: Int32 = 1, pad: Int32 = 0, mask: Int32 = 4

    /// A sequence laid out elsewhere (a reference row).
    public init(ids: [Int32], markers: [Int]) { self.ids = ids; self.markers = markers }

    public init(tokenizer: LayaTokenizer, state: String, question: LayaQuestion,
                window: Int = 256, headMax: Int = 256, optionTokens: Int = 48) {
        let maskText = "<mask>"
        let clean = { (s: String) in s.replacingOccurrences(of: maskText, with: " ") }
        var head = tokenizer.encode("\(question.type) question: \(clean(question.instructions))")
        var options = question.optionTexts.map { text -> [Int32] in
            [Self.mask] + Array(tokenizer.encode(" " + clean(text)).prefix(optionTokens))
        }
        var budget = headMax - options.reduce(0) { $0 + $1.count }
        if budget < 16 {
            let per = max(4, (headMax - 16) / max(1, options.count))
            options = options.map { Array($0.prefix(per)) }
            budget = headMax - options.reduce(0) { $0 + $1.count }
        }
        head = Array(head.prefix(max(8, budget)))
        var ids: [Int32] = [Self.cls] + head + [Self.sep]
        var markers: [Int] = []
        for o in options {
            markers.append(ids.count)
            ids += o
        }
        ids.append(Self.sep)
        let room = max(0, window - ids.count - 1)
        ids += tokenizer.encode(clean(state)).prefix(room)
        ids.append(Self.sep)
        self.ids = Array(ids.prefix(window))
        self.markers = markers.filter { $0 < window }
    }
}
