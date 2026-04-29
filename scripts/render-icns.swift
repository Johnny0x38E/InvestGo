#!/usr/bin/env swift

import AppKit
import Foundation
import ImageIO

guard CommandLine.arguments.count == 3 else {
  fputs("Usage: render-icns.swift <source.png> <output.icns>\n", stderr)
  exit(1)
}

let sourceURL = URL(fileURLWithPath: CommandLine.arguments[1])
let outputURL = URL(fileURLWithPath: CommandLine.arguments[2])

guard let sourceImage = NSImage(contentsOf: sourceURL) else {
  fputs("Failed to load PNG icon source.\n", stderr)
  exit(1)
}

func makeCGImage(size: Int) -> CGImage? {
  let canvasSize = NSSize(width: size, height: size)
  guard let bitmap = NSBitmapImageRep(
    bitmapDataPlanes: nil,
    pixelsWide: size,
    pixelsHigh: size,
    bitsPerSample: 8,
    samplesPerPixel: 4,
    hasAlpha: true,
    isPlanar: false,
    colorSpaceName: .deviceRGB,
    bytesPerRow: 0,
    bitsPerPixel: 0
  ) else {
    return nil
  }

  bitmap.size = canvasSize
  NSGraphicsContext.saveGraphicsState()
  defer { NSGraphicsContext.restoreGraphicsState() }

  guard let context = NSGraphicsContext(bitmapImageRep: bitmap) else {
    return nil
  }
  NSGraphicsContext.current = context
  context.imageInterpolation = .high
  NSColor.clear.set()
  NSBezierPath(rect: NSRect(origin: .zero, size: canvasSize)).fill()
  sourceImage.draw(
    in: NSRect(origin: .zero, size: canvasSize),
    from: NSRect(origin: .zero, size: sourceImage.size),
    operation: .sourceOver,
    fraction: 1
  )
  return bitmap.cgImage
}

try FileManager.default.createDirectory(
  at: outputURL.deletingLastPathComponent(),
  withIntermediateDirectories: true
)

let iconType = "com.apple.icns" as CFString
let sizes = [16, 32, 64, 128, 256, 512, 1024]

guard let destination = CGImageDestinationCreateWithURL(outputURL as CFURL, iconType, sizes.count, nil) else {
  fputs("Failed to create ICNS destination.\n", stderr)
  exit(1)
}

for size in sizes {
  guard let cgImage = makeCGImage(size: size) else {
    fputs("Failed to render icon size \(size).\n", stderr)
    exit(1)
  }
  CGImageDestinationAddImage(destination, cgImage, nil)
}

guard CGImageDestinationFinalize(destination) else {
  fputs("Failed to write ICNS file.\n", stderr)
  exit(1)
}
