// Package web 提供内嵌的前端构建产物。
package web

import "embed"

// Dist 内含 Vite 构建输出，编译后随二进制一起分发。
//
//go:embed all:dist
var Dist embed.FS
