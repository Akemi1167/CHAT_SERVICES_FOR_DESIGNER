package logger

import "unsafe"

// unsafeString mo phong cach Fiber tra chuoi tro thang vao buffer (khong sao chep).
func unsafeString(b []byte) string { return unsafe.String(unsafe.SliceData(b), len(b)) }
