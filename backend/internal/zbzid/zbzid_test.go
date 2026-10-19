package zbzid

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDataRootMigratesLegacy(t *testing.T) {
	cfg := t.TempDir()
	old := filepath.Join(cfg, LegacyName)
	_ = os.MkdirAll(filepath.Join(old, "projects"), 0o755)
	_ = os.WriteFile(filepath.Join(old, "projects", "a.txt"), []byte("x"), 0o644)
	got := DataRoot(cfg)
	if got != filepath.Join(cfg, Name) {
		t.Fatalf("应迁到新目录，得到 %s", got)
	}
	if b, err := os.ReadFile(filepath.Join(got, "projects", "a.txt")); err != nil || string(b) != "x" {
		t.Fatal("迁移后数据丢失")
	}
	if exists(old) {
		t.Fatal("旧目录应已改名")
	}
}

func TestDataRootKeepsExistingNew(t *testing.T) {
	cfg := t.TempDir()
	_ = os.MkdirAll(filepath.Join(cfg, LegacyName), 0o755)
	_ = os.MkdirAll(filepath.Join(cfg, Name), 0o755)
	if got := DataRoot(cfg); got != filepath.Join(cfg, Name) {
		t.Fatalf("新目录已存在时应直接使用，得到 %s", got)
	}
	if !exists(filepath.Join(cfg, LegacyName)) {
		t.Fatal("新目录已存在时不能动旧目录")
	}
}

func TestDataRootFallbackOnRenameFailure(t *testing.T) {
	cfg := t.TempDir()
	_ = os.MkdirAll(filepath.Join(cfg, LegacyName), 0o755)
	rename = func(string, string) error { return errors.New("占用") }
	defer func() { rename = os.Rename }()
	if got := DataRoot(cfg); got != filepath.Join(cfg, LegacyName) {
		t.Fatalf("改名失败应继续用旧目录，得到 %s", got)
	}
}

func TestDataRootFresh(t *testing.T) {
	cfg := t.TempDir()
	if got := DataRoot(cfg); got != filepath.Join(cfg, Name) {
		t.Fatalf("全新安装应用新目录，得到 %s", got)
	}
}
