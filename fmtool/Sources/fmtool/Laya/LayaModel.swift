// Copied from mywant-ios Common/LayaModel.swift (44da2f3): the same Laya the apps decide
// with, so a decision is the same whichever worker takes it. Change both.

import Foundation
#if canImport(CoreAI)
import CoreAI
#endif

/// What Laya says about one question: a probability per label, and the
/// encoder's reading of the whole sequence (pooled_cls, 768 numbers) — what a
/// head fitted on this device's own labels decides from (LayaHead).
public struct LayaReading: Sendable {
    public let labels: [String]
    public let probabilities: [Double]
    public let rawLogits: [Double]
    public let pooled: [Float]
    /// Per label, what the asker saw of it in what was said — 1 when one of
    /// its keywords (its aliases) is in the words, else 0 (the server's, with
    /// the job). How much it counts is the head's to learn (LayaHead.keyword).
    public var cues: [Double] = []
}

/// Laya (the multilingual checkpoint) on this device, through Apple's Core AI:
/// the same code on an iPhone and on a Mac, only the bundle differs (ios/ or
/// macos/ of mlboydaisuke/Laya-Multilingual-CoreAI — an iOS bundle must never
/// be loaded on a Mac). See LayaFiles for where they come from.
///
/// One forward per question: the sequence (LayaSequence), the question's type
/// one-hot; token_logits read at the option markers, divided by the fitted
/// temperature for the type and option count, softmaxed.
public final class LayaModel: @unchecked Sendable {
    public let tokenizer: LayaTokenizer
    private let window: Int
    private let temperatureByOptions: [String: Double]
    private let temperatureByType: [Double]
    private let runner: Any // InferenceFunction, held untyped so this file builds where Core AI does not exist
    private let lock = NSLock()

    public enum Failure: Error, CustomStringConvertible {
        case unavailable, noFunction, noOutput(String)
        public var description: String {
            switch self {
            case .unavailable: return "Core AI is not on this OS (iOS 27 / macOS 27)"
            case .noFunction: return "the bundle has no main function"
            case .noOutput(let n): return "the model gave no \(n)"
            }
        }
    }

    /// Loads the bundle in `folder` (…/wfp16-s256: the .aimodel, tokenizer/,
    /// metadata.json). Seconds: the model is specialised for this device's GPU.
    public init(folder: URL) async throws {
        let meta = (try? JSONSerialization.jsonObject(with: Data(contentsOf: folder.appendingPathComponent("metadata.json")))) as? [String: Any] ?? [:]
        let decision = meta["decision"] as? [String: Any] ?? [:]
        window = decision["window"] as? Int ?? 256
        let cal = decision // source_temperature, temperature, temperature_by_options
        temperatureByOptions = (cal["temperature_by_options"] as? [String: Double]) ?? [:]
        temperatureByType = (cal["temperature"] as? [Double]) ?? [1, 1, 1]
        tokenizer = try LayaTokenizer(contentsOf: folder.appendingPathComponent("tokenizer/tokenizer.json"))
        #if canImport(CoreAI)
        guard #available(iOS 27.0, macOS 27.0, macCatalyst 27.0, *) else { throw Failure.unavailable }
        let bundle = try FileManager.default.contentsOfDirectory(at: folder, includingPropertiesForKeys: nil)
            .first { $0.pathExtension == "aimodel" || $0.pathExtension == "aimodelc" }
        guard let bundle else { throw Failure.noFunction }
        // The GPU: the publisher's Neural Engine specialisation answers wrongly.
        let model = try await AIModel(contentsOf: bundle, options: SpecializationOptions(preferredComputeUnitKind: .gpu))
        guard let main = try model.loadFunction(named: "main") else { throw Failure.noFunction }
        runner = main
        #else
        throw Failure.unavailable
        #endif
    }

    /// The temperature for a question of this type with k options (laya's
    /// buckets: by type and option count first, then by type).
    func temperature(type: String, k: Int) -> Double {
        let bucket: String
        switch type {
        case "choice": bucket = k <= 2 ? "choice:2" : k <= 5 ? "choice:3-5" : k <= 10 ? "choice:6-10" : "choice:11+"
        case "score": bucket = k >= 3 && k <= 5 ? "score:3-5" : "score:\(k)"
        default: bucket = "noul:\(k)"
        }
        if let t = temperatureByOptions[bucket], t > 0 { return t }
        let i = type == "score" ? 1 : type == "noul" ? 2 : 0
        return i < temperatureByType.count && temperatureByType[i] > 0 ? temperatureByType[i] : 1
    }

    /// Laya's answer to one question about `state`.
    public func read(state: String, question: LayaQuestion) async throws -> LayaReading {
        let seq = LayaSequence(tokenizer: tokenizer, state: state, question: question, window: window)
        let (logits, pooled) = try await forward(seq, typeIndex: question.typeIndex)
        let raw = seq.markers.map { Double(logits[$0]) }
        let t = temperature(type: question.type, k: raw.count)
        return LayaReading(labels: Array(question.labels.prefix(raw.count)),
                           probabilities: softmax(raw.map { $0 / t }), rawLogits: raw, pooled: pooled)
    }

    /// The forward pass: token_logits [S] and pooled_cls [768].
    func forward(_ seq: LayaSequence, typeIndex: Int) async throws -> ([Float], [Float]) {
        #if canImport(CoreAI)
        guard #available(iOS 27.0, macOS 27.0, macCatalyst 27.0, *), let main = runner as? InferenceFunction else {
            throw Failure.unavailable
        }
        var ids = [Int32](repeating: LayaSequence.pad, count: window)
        var mask = [Int32](repeating: 0, count: window)
        for (i, id) in seq.ids.enumerated() where i < window { ids[i] = id; mask[i] = 1 }
        var onehot: [Float] = [0, 0, 0]
        onehot[typeIndex] = 1
        var outputs = try await main.run(inputs: [
            "input_ids": NDArray(scalars: ids, shape: [1, window]),
            "attention_mask": NDArray(scalars: mask, shape: [1, window]),
            "qtype_onehot": NDArray(scalars: onehot, shape: [1, 3]),
        ])
        guard let logits = outputs.remove("token_logits")?.ndArray else { throw Failure.noOutput("token_logits") }
        guard let pooled = outputs.remove("pooled_cls")?.ndArray else { throw Failure.noOutput("pooled_cls") }
        return (Self.floats(logits), Self.floats(pooled))
        #else
        throw Failure.unavailable
        #endif
    }

    #if canImport(CoreAI)
    @available(iOS 27.0, macOS 27.0, macCatalyst 27.0, *)
    static func floats(_ a: NDArray) -> [Float] {
        a.view(as: Float.self).withUnsafePointer { p, shape, _ in
            var n = 1
            for i in 0..<shape.count { n *= shape[i] }
            return Array(UnsafeBufferPointer(start: p, count: n))
        }
    }
    #endif
}

func softmax(_ xs: [Double]) -> [Double] {
    guard let m = xs.max() else { return [] }
    let e = xs.map { exp($0 - m) }
    let s = e.reduce(0, +)
    return e.map { $0 / s }
}
