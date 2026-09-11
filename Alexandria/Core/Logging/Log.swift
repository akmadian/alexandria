//
//  Log.swift
//  Alexandria
//
//  Logging setup. swift-log is the facade; call sites make their own
//  `Logger(label:)` (the label becomes the OSLog category). Values are logged
//  public by default — redaction is opt-in at the call site, not the norm.
//

import Foundation
import Logging

nonisolated enum Log {
    static let subsystem = Bundle.main.bundleIdentifier ?? "com.alexandria"

    /// Wire swift-log to OSLog (Xcode/Console, colored by level) and a text
    /// file (readable, public). Call once at launch before anything logs.
    static func bootstrap() {
        LoggingSystem.bootstrap { label in
            MultiplexLogHandler([
                OSLogHandler(label: label)
            ])
        }
    }
}
