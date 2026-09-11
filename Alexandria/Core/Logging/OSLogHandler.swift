//
//  OSLogHandler.swift
//  Alexandria
//
//  Bridges swift-log to os.Logger so messages land in Xcode's console and
//  Console.app, colored by level. Everything is logged `.public`; opt into
//  redaction by masking a value before it reaches the log call.
//

import Logging
import os

struct OSLogHandler: LogHandler {
    let label: String
    private let logger: os.Logger

    var logLevel: Logging.Logger.Level = .debug
    var metadata: Logging.Logger.Metadata = [:]

    init(label: String) {
        self.label = label
        self.logger = os.Logger(subsystem: Log.subsystem, category: label)
    }

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
        logger.log(level: level.osLogType, "\(message.description, privacy: .public)\(fields, privacy: .public)")
    }
}

private extension Logging.Logger.Level {
    var osLogType: OSLogType {
        switch self {
        case .trace, .debug: return .debug
        case .info, .notice: return .info
        case .warning:       return .default
        case .error:         return .error
        case .critical:      return .fault
        }
    }
}
