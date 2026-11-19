package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Setup voi File: log phai xuat hien trong file, va thu muc cha duoc tu tao.
func TestSetupGhiRaFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "app.log")

	_, closer := Setup(Options{Level: "info", Format: "json", File: path, Stdout: true, Service: "chat-service", Env: "local"})
	slog.Info("http request", "method", "POST", "status", 200)
	if err := closer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("doc file log: %v", err)
	}
	got := string(b)
	for _, want := range []string{`"msg":"http request"`, `"service":"chat-service"`, `"env":"local"`, `"status":200`} {
		if !strings.Contains(got, want) {
			t.Errorf("file log thieu %s\n--- noi dung ---\n%s", want, got)
		}
	}
}

// Append: restart khong duoc xoa log cu.
func TestSetupAppendKhongXoaLogCu(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")

	for _, msg := range []string{"lan chay 1", "lan chay 2"} {
		_, closer := Setup(Options{Level: "info", Format: "text", File: path, Stdout: true})
		slog.Info(msg)
		_ = closer.Close()
	}

	b, _ := os.ReadFile(path)
	for _, want := range []string{"lan chay 1", "lan chay 2"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("mat %q sau lan chay thu hai:\n%s", want, b)
		}
	}
}

// File="" => khong tao file nao, closer la noop.
func TestSetupKhongCoFile(t *testing.T) {
	dir := t.TempDir()
	_, closer := Setup(Options{Level: "info", Format: "json", File: "", Stdout: true})
	if err := closer.Close(); err != nil {
		t.Fatalf("noop closer tra loi: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("khong nen tao file nao, co: %v", entries)
	}
}

// Duong dan khong mo duoc => service van chay, chi canh bao (khong panic,
// khong mat log vi van con stdout).
func TestSetupFileLoiThiVanChay(t *testing.T) {
	dir := t.TempDir()
	// Tao mot FILE roi coi no nhu thu muc cha => MkdirAll chac chan loi ENOTDIR.
	blocked := filepath.Join(dir, "la-mot-file")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, closer := Setup(Options{Level: "info", Format: "json", File: filepath.Join(blocked, "app.log"), Stdout: true})
	slog.Info("van ghi duoc ra stdout")
	if err := closer.Close(); err != nil {
		t.Fatalf("phai la noop closer, nhung tra loi: %v", err)
	}
	if b, _ := os.ReadFile(blocked); string(b) != "x" {
		t.Errorf("khong duoc ghi de file dang co, gio la: %q", b)
	}
}

// Stdout=false: log vao file, console khong co dong log nao (tru 1 dong bao
// "log -> <file>" co y in ra de biet duong dan).
func TestSetupStdoutFalseThiConsoleSach(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")

	stdout := captureStdout(t, func() {
		_, closer := Setup(Options{Level: "info", Format: "json", File: path, Stdout: false})
		slog.Info("khong duoc hien ra console", "k", "v")
		_ = closer.Close()
	})

	if strings.Contains(stdout, "khong duoc hien ra console") {
		t.Errorf("log van lot ra stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "log -> "+path) {
		t.Errorf("thieu dong bao duong dan file:\n%s", stdout)
	}
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "khong duoc hien ra console") {
		t.Errorf("file log phai co dong do, err=%v noi dung=%s", err, b)
	}
}

// Stdout=false ma khong co File = mat sach log => phai tu quay ve stdout.
func TestSetupStdoutFalseKhongCoFileThiVanInStdout(t *testing.T) {
	stdout := captureStdout(t, func() {
		_, closer := Setup(Options{Level: "info", Format: "json", File: "", Stdout: false})
		slog.Info("van phai thay dong nay")
		_ = closer.Close()
	})

	if !strings.Contains(stdout, "van phai thay dong nay") {
		t.Errorf("da mat log:\n%s", stdout)
	}
	if !strings.Contains(stdout, "LOG_STDOUT=false nhung khong co LOG_FILE") {
		t.Errorf("thieu canh bao:\n%s", stdout)
	}
}

// captureStdout doi os.Stdout sang pipe trong luc chay fn.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		_, _ = io.Copy(&sb, r)
		done <- sb.String()
	}()

	fn()

	os.Stdout = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}
