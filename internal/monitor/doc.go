// Package monitor 实现桌面端投资观察台的核心领域能力。
//
// 这里有三个边界需要特别注意：
// 1. Store 负责协调持久化状态、实时行情和提醒规则；
// 2. API 只做 HTTP 输入输出转换，尽量保持薄；
// 3. 外部行情源通过 provider/service 隔离，避免直接渗透到 Store。
package monitor
