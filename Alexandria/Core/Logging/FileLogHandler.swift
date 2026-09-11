//
//  FileLogHandler.swift
//  Alexandria
//
//  Appends plain-text log lines to a single file in Application Support. Every
//  handler instance writes through one shared sink (serial queue, one file
//  handle) so lines from different categories never interleave or corrupt.
//

import Logging
import Foundation

struct FileLogHandler: LogHandler {
    let label: String
    var logLevel: Logging.Logger.Level = .debug
    var metadata: Logging.Logger.Metadata = [:]

    init(label: String) { self.label = label }

    subscript(metadataKey key: String) -> Logging.Logger.Metadata.Value? {
        get { metadata[key] }
        set { metadata[key] = newValue }
    }

    func log(level: Logging.Logger.Level,
             message: Logging.Logger.Message,
             metadata: Logging.Logger.Metadata?,
             source: String, file: String, function: String, line: UInt) {
        let combined = self.metadata.merging(metadata ?? [:]) { $1 }
        let fields = combined.isEmpty
            ? ""
            : " " + combined.map { "\($0)=\($1)" }.joined(separator: " ")
        let stamp = FileSink.timestamp.string(from: Date())
        FileSink.shared.write("\(stamp) [\(level)] \(label): \(message)\(fields)\n")
    }
}

/// One handle, one serial queue, shared by every FileLogHandler.
final class FileSink {
    static let shared = FileSink()

    static let timestamp: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    static var fileURL: URL {
        let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        return support.appendingPathComponent("Alexandria/Logs/alexandria.log")
    }

    private let queue = DispatchQueue(label: "com.alexandria.log.file")
    private let handle: FileHandle?

    private init() {
        let url = FileSink.fileURL
        let manager = FileManager.default
        try? manager.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        if !manager.fileExists(atPath: url.path) {
            manager.createFile(atPath: url.path, contents: nil)
        }
        let opened = try? FileHandle(forWritingTo: url)
        try? opened?.seekToEnd()
        self.handle = opened
    }

    func write(_ line: String) {
        // ponytail: single file, no rotation. Swap in Puppy's RotationLogger if it grows unbounded.
        queue.async { [handle] in try? handle?.write(contentsOf: Data(line.utf8)) }
    }
}
