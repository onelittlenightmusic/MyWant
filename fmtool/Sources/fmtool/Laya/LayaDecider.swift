// Copied from mywant-ios Common/LayaDecider.swift (44da2f3): the same Laya the apps decide
// with, so a decision is the same whichever worker takes it. Change both.

import Foundation

/// The robot's typed decisions on this device — the model work `decide` and
/// `train-decider` (engine/server/model_jobs.go, robot_initiative_learn.go).
///
/// The server keeps what the robot met, what came of it, and the heads fitted
/// on that, and hands them over with each job; the model is here: Laya through
/// Core AI (LayaModel). One worker's training (LayaHead, fitted here) serves
/// every worker, since all read alike — the iPhone, the Mac, a GPU server's
/// tools/laya_worker.py. Nothing is kept on the device but the model.
///
///   decide        {"items": [{"state", "questions": {id: question}}]}  (or one {"state", "questions"})
///              →  {"items": [{"answers": {id: {"choice", "probabilities": {label: p}}}, "ms", "heads": [id…]}]}
///   train-decider {"rows": [{"state", "gold": {id: label}}], "questions"}
///              →  {"rows", "fit_seconds", "serving": [id…], "held_out": {id: {"head", "laya"}}}
public actor LayaDecider {
    public static let shared = LayaDecider()

    public static let decideOp = "decide"
    public static let trainOp = "train-decider"

    /// Who took the work, as the server's record says it.
    public nonisolated static var workerName: String {
        ProcessInfo.processInfo.environment["LAYA_WORKER"] ?? "app-\(LayaFiles.variant.hasPrefix("ios") ? "iphone" : "mac")"
    }

    private var model: LayaModel?
    private var loading: Task<LayaModel, any Error>?

    /// Whether this device can take the robot's decisions: Core AI is here and
    /// the model is on disk (LayaFiles fetches it when asked to).
    public nonisolated static var available: Bool {
        guard #available(iOS 27.0, macOS 27.0, macCatalyst 27.0, *) else { return false }
        return LayaFiles.ready
    }

    private func loaded() async throws -> LayaModel {
        if let model { return model }
        if let loading { return try await loading.value }
        let task = Task { try await LayaModel(folder: LayaFiles.folder) }
        loading = task
        defer { loading = nil }
        let m = try await task.value
        model = m
        return m
    }

    /// A question's head is kept under what the question is: its id, type,
    /// instructions and labels — a question worded otherwise is another one.
    static func headKey(_ id: String, _ q: LayaQuestion) -> String {
        "\(id)|\(q.type)|\(q.instructions)|\(q.labels.joined(separator: ","))"
    }

    // ── decide ──

    /// `heads` come with the job, from the server (whatever worker fitted
    /// them): this device keeps none of its own.
    public func decide(state: String, questions: [String: LayaQuestion],
                       heads: [String: LayaHead] = [:]) async throws -> [String: Any] {
        let model = try await loaded()
        let started = Date()
        var answers: [String: Any] = [:]
        var byHead: [String] = []
        for (id, q) in questions.sorted(by: { $0.key < $1.key }) {
            let reading = try await model.read(state: state, question: q)
            var probs = reading.probabilities
            if let head = heads[Self.headKey(id, q)], head.serving, head.labels == reading.labels {
                probs = head.probabilities(reading)
                byHead.append(id)
            }
            let best = probs.indices.max(by: { probs[$0] < probs[$1] }) ?? 0
            answers[id] = [
                "choice": reading.labels.indices.contains(best) ? reading.labels[best] : "",
                "probabilities": Dictionary(uniqueKeysWithValues: zip(reading.labels, probs)),
            ]
        }
        return ["answers": answers, "ms": Int(Date().timeIntervalSince(started) * 1000), "heads": byHead]
    }

    // ── train ──

    public struct Row: Decodable, Sendable {
        public let state: String
        public let gold: [String: String]
    }

    /// Fits the heads and hands them back, for the server to keep and send
    /// with every decision — whichever worker takes it then.
    public func train(rows: [Row], questions: [String: LayaQuestion]) async throws -> [String: Any] {
        let model = try await loaded()
        let started = Date()
        var serving: [String] = []
        var heldOut: [String: Any] = [:]
        var fitted: [String: LayaHead] = [:]
        for (id, q) in questions.sorted(by: { $0.key < $1.key }) {
            let labelled = rows.filter { $0.gold[id] != nil }
            var readings: [LayaReading] = []
            var gold: [String] = []
            let states = labelled.map(\.state)
            for row in labelled {
                // Each read awaits the model: a decision asked meanwhile is
                // run between them.
                readings.append(try await model.read(state: row.state, question: q))
                gold.append(row.gold[id]!)
            }
            let labels = Array(q.labels.prefix(readings.first?.labels.count ?? q.labels.count))
            // The fitting is seconds of arithmetic: off this actor, so the
            // decisions it serves are not held up behind it.
            let head = await Task.detached(priority: .utility) {
                LayaHead.fit(readings: readings, gold: gold, labels: labels, states: states)
            }.value
            guard let head else { continue }
            fitted[Self.headKey(id, q)] = head
            heldOut[id] = ["head": head.heldOutAccuracy, "laya": head.layaHeldOutAccuracy, "rows": labelled.count]
            if head.serving { serving.append(id) }
        }
        let encoded = (try? JSONEncoder().encode(fitted)).flatMap { try? JSONSerialization.jsonObject(with: $0) } ?? [:]
        return ["rows": rows.count, "fit_seconds": Int(Date().timeIntervalSince(started)),
                "serving": serving, "held_out": heldOut, "worker": Self.workerName, "heads": encoded]
    }

    // ── as model work ──

    /// `run`, its output as JSON: what crosses out of this actor to fmtool's
    /// serve loop (Laya.swift) — Data is Sendable, Any is not. fmtool's own.
    public func runJSON(op: String, input: Data) async -> (output: Data?, error: String?) {
        let (output, error) = await run(op: op, input: input)
        if let error { return (nil, error) }
        guard let output, JSONSerialization.isValidJSONObject(output),
              let data = try? JSONSerialization.data(withJSONObject: output) else {
            return (nil, "laya: the output is not JSON")
        }
        return (data, nil)
    }

    /// A job's input run as `op`, its output as the server takes it back.
    public func run(op: String, input: Data) async -> (output: Any?, error: String?) {
        struct Item: Decodable { let state: String; let questions: [String: LayaQuestion] }
        // One state, or several at once ({"items": [...]}, the server's form).
        struct DecideIn: Decodable {
            let state: String?; let questions: [String: LayaQuestion]?; let items: [Item]?
            let heads: [String: LayaHead]?
        }
        struct TrainIn: Decodable { let rows: [Row]?; let questions: [String: LayaQuestion] }
        do {
            switch op {
            case Self.decideOp:
                let req = try JSONDecoder().decode(DecideIn.self, from: input)
                if let items = req.items {
                    var out: [[String: Any]] = []
                    for item in items {
                        out.append(try await decide(state: item.state, questions: item.questions, heads: req.heads ?? [:]))
                    }
                    return (["items": out, "worker": Self.workerName], nil)
                }
                return (try await decide(state: req.state ?? "", questions: req.questions ?? [:], heads: req.heads ?? [:]), nil)
            case Self.trainOp:
                let req = try JSONDecoder().decode(TrainIn.self, from: input)
                return (try await train(rows: req.rows ?? [], questions: req.questions), nil)
            default:
                return (nil, "this device cannot do \(op)")
            }
        } catch {
            return (nil, "laya: \(error)")
        }
    }
}
