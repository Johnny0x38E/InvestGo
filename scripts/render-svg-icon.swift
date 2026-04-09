#!/usr/bin/env swift

import AppKit
import Foundation

guard CommandLine.arguments.count == 4 else {
  fputs("Usage: render-svg-icon.swift <source.svg> <output.png> <size>\n", stderr)
  exit(1)
}

let sourceURL = URL(fileURLWithPath: CommandLine.arguments[1])
let outputURL = URL(fileURLWithPath: CommandLine.arguments[2])

guard let dimension = Double(CommandLine.arguments[3]), dimension > 0 else {
  fputs("Icon size must be a positive number.\n", stderr)
  exit(1)
}

guard let image = NSImage(contentsOf: sourceURL) else {
  fputs("Failed to load SVG icon source.\n", stderr)
  exit(1)
}

let canvasSize = NSSize(width: dimension, height: dimension)
let pixelsWide = Int(dimension.rounded())
let pixelsHigh = Int(dimension.rounded())

guard let bitmap = NSBitmapImageRep(
  bitmapDataPlanes: nil,
  pixelsWide: pixelsWide,
  pixelsHigh: pixelsHigh,
  bitsPerSample: 8,
  samplesPerPixel: 4,
  hasAlpha: true,
  isPlanar: false,
  colorSpaceName: .deviceRGB,
  bytesPerRow: 0,
  bitsPerPixel: 0
) else {
  fputs("Failed to allocate icon bitmap.\n", stderr)
  exit(1)
}

bitmap.size = canvasSize

NSGraphicsContext.saveGraphicsState()
guard let context = NSGraphicsContext(bitmapImageRep: bitmap) else {
  fputs("Failed to create bitmap graphics context.\n", stderr)
  exit(1)
}

NSGraphicsContext.current = context
context.imageInterpolation = .high
NSColor.clear.set()
NSBezierPath(rect: NSRect(origin: .zero, size: canvasSize)).fill()
image.draw(
  in: NSRect(origin: .zero, size: canvasSize),
  from: NSRect(origin: .zero, size: image.size),
  operation: .sourceOver,
  fraction: 1
)
NSGraphicsContext.restoreGraphicsState()

guard let pngData = bitmap.representation(using: .png, properties: [:]) else {
  fputs("Failed to encode PNG icon.\n", stderr)
  exit(1)
}

try FileManager.default.createDirectory(
  at: outputURL.deletingLastPathComponent(),
  withIntermediateDirectories: true
)
try pngData.write(to: outputURL)
