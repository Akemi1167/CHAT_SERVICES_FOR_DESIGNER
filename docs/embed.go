// Package docs nhung tai lieu vao binary de phuc vu qua HTTP (GET /docs/flow).
//
// Image prod chi chua binary, khong co thu muc docs/ — nen phai embed thay vi
// doc file luc chay. Them file moi: nho mo ca trong .dockerignore.
package docs

import _ "embed"

// Flow la noi dung docs/FLOW.md tai thoi diem build.
//
//go:embed FLOW.md
var Flow []byte
