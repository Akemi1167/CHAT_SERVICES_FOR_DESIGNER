package config

import (
	"os"
	"testing"
	"time"
)

// Docker compose truyen gia tri kem khoang trang dem truoc dau "#".
// Neu khong TrimSpace thi ParseBool/Atoi loi va cau hinh bi bo qua am tham.
func TestDocBienCoKhoangTrangThua(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want any
		got  func() any
	}{
		{"boolean co khoang trang duoi", "true        ", true, func() any { return boolean("T_BOOL", false) }},
		{"boolean co khoang trang truoc", "   true", true, func() any { return boolean("T_BOOL", false) }},
		{"num co khoang trang", "25   ", 25, func() any { return num("T_BOOL", 1) }},
		{"duration co khoang trang", "300ms  ", 300 * time.Millisecond, func() any { return dur("T_BOOL", time.Second) }},
		{"str co khoang trang", "  chat:events ", "chat:events", func() any { return str("T_BOOL", "x") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("T_BOOL", c.raw)
			if got := c.got(); got != c.want {
				t.Errorf("gia tri %q => %v, muon %v", c.raw, got, c.want)
			}
		})
	}
}

// Gia tri rac thi van rot ve mac dinh (khong panic).
func TestGiaTriRacThiDungMacDinh(t *testing.T) {
	t.Setenv("T_BAD", "khong-phai-bool")
	if boolean("T_BAD", true) != true {
		t.Error("phai giu mac dinh khi gia tri khong parse duoc")
	}
	os.Unsetenv("T_BAD")
	if boolean("T_BAD", true) != true {
		t.Error("khong set thi phai dung mac dinh")
	}
}
