// Package zbzid 是章八爪的软件身份（目录名、程序名）与旧章八爪身份的一次性迁移。作者：长鱼茶汤茗。
// 零依赖，可被任何底层包引用。上游文件不直接改，由 scripts/zbz/identity.py 在构建时接入。
package zbzid

import (
	"os"
	"path/filepath"
)

const (
	// Name 是数据目录、安装目录、缓存目录使用的英文身份名。
	Name = "ZhangBaZhua"
	// ExeName 是 Windows 主程序文件名。
	ExeName = "ZhangBaZhua.exe"
	// LegacyName 是上游章八爪使用的目录名，只用于迁移。
	LegacyName = "BeefTV"
)

var rename = os.Rename

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// migrate 把旧目录整体改名为新目录；返回最终应使用的目录。
// 规则：新目录已存在 → 用新目录；只有旧目录 → 改名，失败则继续用旧目录（绝不新建空工作区）；都不存在 → 新目录。
func migrate(legacy, target string) string {
	if exists(target) {
		return target
	}
	if !exists(legacy) {
		return target
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return legacy
	}
	if err := rename(legacy, target); err != nil {
		return legacy
	}
	return target
}

// DataRoot 返回章八爪的数据根目录（configDir 一般是 %APPDATA%），首次运行时从章八爪目录迁移。
func DataRoot(configDir string) string {
	return migrate(filepath.Join(configDir, LegacyName), filepath.Join(configDir, Name))
}

// WebviewDir 返回 WebView2 用户数据目录（localStorage：关注列表、外观、模块开关）。
// Wails 默认用 %APPDATA%\<exe 名>，旧版即 %APPDATA%\BeefTV.exe；首次运行时迁到 %APPDATA%\ZhangBaZhua\WebView2。
// 必须在数据根迁移之后调用。无法定位时返回空串，交给 Wails 默认行为。
func WebviewDir() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	root := DataRoot(cfg)
	if filepath.Base(root) != Name {
		// 数据根迁移失败、仍在用旧目录时，WebView2 也保持旧路径，避免设置分家。
		return filepath.Join(cfg, LegacyName+".exe")
	}
	return migrate(filepath.Join(cfg, LegacyName+".exe"), filepath.Join(root, "WebView2"))
}

// CacheDir 返回更新缓存等可丢弃数据的目录名。
func CacheDir(cacheRoot string) string {
	return filepath.Join(cacheRoot, Name)
}
