package main

import "embed"

// staticFiles 嵌入前端构建产物（运行 `npm run build` 后生成）
// 编译时需确保 static/ 目录存在
//
//go:embed static
var staticFiles embed.FS
