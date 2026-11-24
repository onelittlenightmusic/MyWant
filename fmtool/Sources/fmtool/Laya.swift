import Foundation

/// Laya's work for a MyWant server on this Mac — a decision, or a training of
/// the decision's heads — taken as a device's worker takes it (the server's
/// model job: op and input in, output back), with the apps' own Laya code
/// (Laya/, copied from mywant-ios) on the bundle the Mac app fetched
/// (~/Library/Application Support/Laya/macos/wfp16-s256). The server hands it
/// here only when no device has taken it (engine/server robot_initiative.go,
/// layaJob): a device first, as for Apple's model.
func runLaya(op: String, input: String) async -> (output: Any?, error: String?) {
    guard LayaDecider.available else {
        return (nil, "Laya is not on this Mac (no Core AI, or the Mac app has not fetched its bundle)")
    }
    let (data, error) = await LayaDecider.shared.runJSON(op: op, input: Data(input.utf8))
    if let error { return (nil, error) }
    return (data.flatMap { try? JSONSerialization.jsonObject(with: $0) }, nil)
}
