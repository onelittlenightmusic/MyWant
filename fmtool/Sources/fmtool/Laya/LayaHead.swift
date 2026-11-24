// Copied from mywant-ios Common/LayaHead.swift (44da2f3): the same Laya the apps decide
// with, so a decision is the same whichever worker takes it. Change both.

import Foundation

/// A head fitted on this device for one question: softmax regression over what
/// Laya read (pooled_cls, its 768 numbers, and its own probabilities for the
/// options), from labels the server keeps and hands over (train-decider).
///
/// Laya as it ships reads the words; what the person wants said is theirs,
/// and a few hundred labelled situations teach it. Small enough to fit in a
/// second on a phone and keep as JSON; the same on an iPhone and a Mac, since
/// the encoder under it is the same.
public struct LayaHead: Codable, Sendable {
    public let labels: [String]
    let mean: [Float]
    let scale: [Float]
    let weights: [[Float]] // [label][feature]
    let bias: [Float]
    /// Balanced accuracy in five-fold cross-validation, the head's and Laya's own.
    public let heldOutAccuracy: Double
    public let layaHeldOutAccuracy: Double
    /// Served only when it did better than Laya on the rows it never saw.
    public var serving: Bool { heldOutAccuracy > layaHeldOutAccuracy }

    static func features(_ r: LayaReading) -> [Float] { r.pooled + r.probabilities.map { Float($0) } }

    /// The head's probabilities for a reading.
    public func probabilities(_ r: LayaReading) -> [Double] {
        let x = Self.features(r)
        guard x.count == mean.count else { return r.probabilities }
        let z = zip(zip(x, mean), scale).map { ($0.0 - $0.1) * $1 }
        var logits: [Double] = []
        for k in weights.indices {
            var sum = bias[k]
            for j in 0..<z.count { sum += weights[k][j] * z[j] }
            logits.append(Double(sum))
        }
        return softmax(logits)
    }

    /// Fits a head on readings and their gold labels: a fifth held out to say
    /// how it does against Laya alone, then fitted again on all of them.
    /// The fold a row falls in: by its words (FNV-1a over UTF-8, mod 5), so the
    /// copies of one row (a recent reply counts five times) fall together and
    /// none is scored by a head that saw it. tools/laya_worker.py folds alike.
    static func fold(_ state: String) -> Int {
        var h: UInt32 = 2_166_136_261
        for b in state.utf8 { h = (h ^ UInt32(b)) &* 16_777_619 }
        return Int(h % 5)
    }

    /// Fits a head on readings and their gold labels. Whether it serves is
    /// settled by five-fold cross-validation — each fold scored by a head fitted
    /// on the other four, every row scored once — against Laya alone; then it
    /// is fitted again on all of them.
    public static func fit(readings: [LayaReading], gold: [String], labels: [String], states: [String]) -> LayaHead? {
        let rows = zip(zip(readings, gold), states).compactMap { pair, state -> (LayaReading, Int, Int)? in
            labels.firstIndex(of: pair.1).map { (pair.0, $0, fold(state)) }
        }
        guard rows.count >= 10, Set(rows.map(\.1)).count >= 2 else { return nil }
        var headPicks: [(picked: Int, gold: Int)] = [], layaPicks: [(picked: Int, gold: Int)] = []
        for k in 0..<5 {
            let test = rows.filter { $0.2 == k }
            let fitRows = rows.filter { $0.2 != k }.map { ($0.0, $0.1) }
            guard !test.isEmpty, Set(fitRows.map(\.1)).count >= 2 else { continue }
            let trial = train(fitRows, labels: labels)
            for r in test {
                headPicks.append((argmax(trial.probabilities(r.0)), r.1))
                layaPicks.append((argmax(r.0.probabilities), r.1))
            }
        }
        let final = train(rows.map { ($0.0, $0.1) }, labels: labels)
        return LayaHead(labels: labels, mean: final.mean, scale: final.scale, weights: final.weights,
                        bias: final.bias, heldOutAccuracy: balanced(headPicks), layaHeldOutAccuracy: balanced(layaPicks))
    }

    static func argmax(_ ps: [Double]) -> Int { ps.indices.max(by: { ps[$0] < ps[$1] }) ?? 0 }

    /// The mean of each label's own accuracy — with most rows one label (an
    /// offer is mostly declined), plain accuracy praises a model that always
    /// says it.
    static func balanced(_ picks: [(picked: Int, gold: Int)]) -> Double {
        var hit: [Int: Int] = [:], all: [Int: Int] = [:]
        for p in picks {
            all[p.gold, default: 0] += 1
            if p.picked == p.gold { hit[p.gold, default: 0] += 1 }
        }
        guard !all.isEmpty else { return 0 }
        return all.keys.map { Double(hit[$0] ?? 0) / Double(all[$0]!) }.reduce(0, +) / Double(all.count)
    }

    /// Full-batch gradient descent (Adam) on the cross-entropy, L2 on the weights.
    private static func train(_ rows: [(LayaReading, Int)], labels: [String]) -> LayaHead {
        let xs = rows.map { features($0.0) }
        let d = xs.first?.count ?? 0, k = labels.count, n = Float(rows.count)
        var mean = [Float](repeating: 0, count: d), scale = [Float](repeating: 1, count: d)
        for j in 0..<d {
            let col = xs.map { $0[j] }
            let m = col.reduce(0, +) / n
            let v = col.reduce(0) { $0 + ($1 - m) * ($1 - m) } / n
            mean[j] = m
            scale[j] = v > 1e-8 ? 1 / v.squareRoot() : 1
        }
        let zs = xs.map { x in (0..<d).map { (x[$0] - mean[$0]) * scale[$0] } }
        var w = [[Float]](repeating: [Float](repeating: 0, count: d), count: k)
        var b = [Float](repeating: 0, count: k)
        var mw = w, vw = w, mb = b, vb = b
        let lr: Float = 0.05, l2: Float = 1e-2, b1: Float = 0.9, b2: Float = 0.999
        for step in 1...300 {
            var gw = [[Float]](repeating: [Float](repeating: 0, count: d), count: k)
            var gb = [Float](repeating: 0, count: k)
            for (i, z) in zs.enumerated() {
                let logits = (0..<k).map { c in zip(w[c], z).reduce(b[c]) { $0 + $1.0 * $1.1 } }
                let p = softmax(logits.map(Double.init)).map(Float.init)
                for c in 0..<k {
                    let g = (p[c] - (c == rows[i].1 ? 1 : 0)) / n
                    gb[c] += g
                    for j in 0..<d { gw[c][j] += g * z[j] }
                }
            }
            let t = Float(step)
            for c in 0..<k {
                for j in 0..<d {
                    let g = gw[c][j] + l2 * w[c][j]
                    mw[c][j] = b1 * mw[c][j] + (1 - b1) * g
                    vw[c][j] = b2 * vw[c][j] + (1 - b2) * g * g
                    w[c][j] -= lr * (mw[c][j] / (1 - pow(b1, t))) / ((vw[c][j] / (1 - pow(b2, t))).squareRoot() + 1e-8)
                }
                mb[c] = b1 * mb[c] + (1 - b1) * gb[c]
                vb[c] = b2 * vb[c] + (1 - b2) * gb[c] * gb[c]
                b[c] -= lr * (mb[c] / (1 - pow(b1, t))) / ((vb[c] / (1 - pow(b2, t))).squareRoot() + 1e-8)
            }
        }
        return LayaHead(labels: labels, mean: mean, scale: scale, weights: w, bias: b,
                        heldOutAccuracy: 0, layaHeldOutAccuracy: 0)
    }
}
