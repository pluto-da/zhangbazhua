#!/usr/bin/env python3
"""章八爪模块 · 软件身份统一（构建时补丁）· 作者 长鱼茶汤茗
把用户能看到的身份触点从 BeefTV 换成 章八爪 / ZhangBaZhua：安装目录、主程序名、开始菜单、卸载注册表键、
安装日志、数据目录（带迁移）、WebView2 目录（带迁移）、更新缓存、版权字段、安装器文案。
- 只在 CI 构建时运行，不提交到 custom 分支，跟随上游合并不冲突。
- 每条替换都断言命中；上游改了被补丁的代码，构建就失败并说明原因，绝不悄悄漏改。
- 保留（C 类内部标识，正常使用看不到）：cli\\beeftv.exe（AI 助手按命令名调用）、.beeftv-plugin 扩展名、互斥锁名、源码包名与测试。
用法：python scripts/zbz/identity.py [--check]   --check 只审计（补丁后应为 0 处遗留），有遗留返回 1。
必须在 rebrand.py 之前运行（rebrand 会改写部分文案）。
"""
import pathlib, re, sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
NEW, EXE, CN = "ZhangBaZhua", "ZhangBaZhua.exe", "章八爪"
OLD_KEY = r'"Software\Microsoft\Windows\CurrentVersion\Uninstall\BeefTV"'
NEW_KEY = r'"Software\Microsoft\Windows\CurrentVersion\Uninstall\ZhangBaZhua"'

# (文件, 原文, 新文, 期望命中次数；None 表示 ≥1 全部替换)
PATCHES = [
    # —— Go：数据目录、WebView2、对话框、更新 ——
    ("backend/internal/desktopstorage/location.go", '\t"strings"\n)', '\t"strings"\n\n\t"infinite-canvas/backend/internal/zbzid"\n)', 1),
    ("backend/internal/desktopstorage/location.go", 'return filepath.Join(root, "BeefTV"), nil', 'return zbzid.DataRoot(root), nil', 1),
    ("backend/cmd/desktop/main.go", '\t"infinite-canvas/backend/internal/desktopupdate"\n', '\t"infinite-canvas/backend/internal/desktopupdate"\n\t"infinite-canvas/backend/internal/zbzid"\n', 1),
    ("backend/cmd/desktop/main.go", "\t\tWindows: &windows.Options{\n", "\t\tWindows: &windows.Options{\n\t\t\tWebviewUserDataPath: zbzid.WebviewDir(),\n", 1),
    ("backend/cmd/desktop/storage.go", 'Title: "选择 BeefTV 数据存储文件夹"', 'Title: "选择章八爪数据存储文件夹"', 1),
    ("backend/cmd/desktop/storage.go", 'path = filepath.Join(path, "BeefTV")', 'path = filepath.Join(path, "ZhangBaZhua")', 1),
    ("backend/internal/desktopupdate/config.go", 'windowsExeName  = "BeefTV.exe"', 'windowsExeName  = "ZhangBaZhua.exe"', 1),
    ("backend/internal/desktopupdate/engine.go", 'filepath.Join(cache, "BeefTV", "updates")', 'filepath.Join(cache, "ZhangBaZhua", "updates")', 1),
    # —— 构建产物名 ——
    ("backend/cmd/desktop/wails.json", '"name": "BeefTV"', '"name": "ZhangBaZhua"', 1),
    ("backend/cmd/desktop/wails.json", '"outputfilename": "BeefTV"', '"outputfilename": "ZhangBaZhua"', 1),
    ("scripts/build-beeftv-windows-release.ps1", '$exePath = Join-Path $binDir "BeefTV.exe"', '$exePath = Join-Path $binDir "ZhangBaZhua.exe"', 1),
    ("scripts/build-beeftv-windows-release.ps1", "%AppData%\\BeefTV", "%AppData%\\ZhangBaZhua", 1),
    ("scripts/package-desktop-installer.py", "'BeefTV.exe' if platform == 'windows-amd64'", "'ZhangBaZhua.exe' if platform == 'windows-amd64'", 1),
    # —— NSIS 安装器 ——
    ("scripts/installers/windows.nsi", 'InstallDir "$LOCALAPPDATA\\Programs\\BeefTV"', 'InstallDir "$LOCALAPPDATA\\Programs\\ZhangBaZhua"', 1),
    ("scripts/installers/windows.nsi", OLD_KEY, NEW_KEY, None),  # 全部换成新键，旧键的兼容在下面单独加
    ("scripts/installers/windows.nsi", 'VIAddVersionKey "LegalCopyright" "BeefTV"', 'VIAddVersionKey "LegalCopyright" "长鱼茶汤茗（基于 BeefTV，MIT）"', 1),
    ("scripts/installers/windows.nsi", '!define MUI_FINISHPAGE_RUN "$INSTDIR\\BeefTV.exe"', '!define MUI_FINISHPAGE_RUN "$INSTDIR\\ZhangBaZhua.exe"', 1),
    ("scripts/installers/windows.nsi", 'StrCpy $INSTDIR "$LOCALAPPDATA\\Programs\\BeefTV"', 'StrCpy $INSTDIR "$LOCALAPPDATA\\Programs\\ZhangBaZhua"', 1),
    ("scripts/installers/windows.nsi", 'StrCpy $1 "$1\\BeefTV"', 'StrCpy $1 "$1\\ZhangBaZhua"', 2),
    ("scripts/installers/windows.nsi", '${If} $2 != "BeefTV"', '${If} $2 != "ZhangBaZhua"', 1),
    ("scripts/installers/windows.nsi", '    ${AndIfNot} ${FileExists} "$1\\BeefTV.exe"\n',
     '    ${AndIfNot} ${FileExists} "$1\\BeefTV.exe"\n    ${AndIfNot} ${FileExists} "$1\\ZhangBaZhua.exe"\n', 1),
    ("scripts/installers/windows.nsi", '\'"$SYSDIR\\tasklist.exe" /FI "IMAGENAME eq BeefTV.exe" /NH\'', '\'"$SYSDIR\\tasklist.exe" /NH\'', 1),
    ("scripts/installers/windows.nsi", '    ${StrStr} $2 $1 "beeftv.exe"\n!endif\n  ${EndIf}\n',
     '    ${StrStr} $2 $1 "beeftv.exe"\n!endif\n  ${EndIf}\n  ${If} $2 == ""\n!if "${PREFIX}" == "un."\n    ${UnStrStr} $2 $1 "ZhangBaZhua.exe"\n!else\n    ${StrStr} $2 $1 "ZhangBaZhua.exe"\n!endif\n  ${EndIf}\n', 1),
    ("scripts/installers/windows.nsi", "/IM BeefTV.exe'", "/IM BeefTV.exe /IM ZhangBaZhua.exe'", 1),
    ("scripts/installers/windows.nsi", "/F /IM BeefTV.exe /T'", "/F /IM BeefTV.exe /IM ZhangBaZhua.exe /T'", 1),
    ("scripts/installers/windows.nsi", '  Delete "$CleanDir\\BeefTV.exe"\n', '  Delete "$CleanDir\\BeefTV.exe"\n  Delete "$CleanDir\\ZhangBaZhua.exe"\n', 1),
    ("scripts/installers/windows.nsi", '"$LOCALAPPDATA\\BeefTV\\Installer"', '"$LOCALAPPDATA\\ZhangBaZhua\\Installer"', 2),
    ("scripts/installers/windows.nsi", 'Section "BeefTV (required)" SEC_APP', 'Section "章八爪 (required)" SEC_APP', 1),
    # 老用户：新键没有时，从旧键找到旧安装目录，安装成功后清理旧载荷
    ("scripts/installers/windows.nsi", f'  ReadRegStr $PreviousInstallDir HKCU {NEW_KEY} "InstallLocation"\n',
     f'  ReadRegStr $PreviousInstallDir HKCU {NEW_KEY} "InstallLocation"\n  ${{If}} $PreviousInstallDir == ""\n    ReadRegStr $PreviousInstallDir HKCU {OLD_KEY} "InstallLocation"\n  ${{EndIf}}\n', 1),
    ("scripts/installers/windows.nsi",
     '  CreateDirectory "$SMPROGRAMS\\BeefTV"\n  Delete "$DESKTOP\\BeefTV.lnk"\n  Delete "$SMPROGRAMS\\BeefTV\\BeefTV.lnk"\n'
     '  CreateShortcut "$SMPROGRAMS\\BeefTV\\章八爪.lnk" "$INSTDIR\\BeefTV.exe"\n  CreateShortcut "$SMPROGRAMS\\BeefTV\\Uninstall.lnk" "$INSTDIR\\Uninstall.exe"\n'
     '  CreateShortcut "$DESKTOP\\章八爪.lnk" "$INSTDIR\\BeefTV.exe"\n',
     '  Delete "$DESKTOP\\BeefTV.lnk"\n  Delete "$SMPROGRAMS\\BeefTV\\BeefTV.lnk"\n  Delete "$SMPROGRAMS\\BeefTV\\章八爪.lnk"\n  Delete "$SMPROGRAMS\\BeefTV\\Uninstall.lnk"\n  RMDir "$SMPROGRAMS\\BeefTV"\n'
     '  CreateDirectory "$SMPROGRAMS\\章八爪"\n  CreateShortcut "$SMPROGRAMS\\章八爪\\章八爪.lnk" "$INSTDIR\\ZhangBaZhua.exe"\n  CreateShortcut "$SMPROGRAMS\\章八爪\\卸载章八爪.lnk" "$INSTDIR\\Uninstall.exe"\n'
     '  CreateShortcut "$DESKTOP\\章八爪.lnk" "$INSTDIR\\ZhangBaZhua.exe"\n', 1),
    ("scripts/installers/windows.nsi", '"DisplayIcon" "$INSTDIR\\BeefTV.exe"', '"DisplayIcon" "$INSTDIR\\ZhangBaZhua.exe"', 1),
    ("scripts/installers/windows.nsi", '  # A successful move replaces the registered install; remove only the old\n',
     f'  # 章八爪：旧 BeefTV 身份的卸载登记一并移除，“应用和功能”里只剩章八爪。\n  DeleteRegKey HKCU {OLD_KEY}\n  # A successful move replaces the registered install; remove only the old\n', 1),
    ("scripts/installers/windows.nsi",
     '    Delete "$SMPROGRAMS\\BeefTV\\BeefTV.lnk"\n    Delete "$SMPROGRAMS\\BeefTV\\章八爪.lnk"\n    Delete "$SMPROGRAMS\\BeefTV\\Uninstall.lnk"\n    RMDir "$SMPROGRAMS\\BeefTV"\n',
     '    Delete "$SMPROGRAMS\\BeefTV\\BeefTV.lnk"\n    Delete "$SMPROGRAMS\\BeefTV\\章八爪.lnk"\n    Delete "$SMPROGRAMS\\BeefTV\\Uninstall.lnk"\n    RMDir "$SMPROGRAMS\\BeefTV"\n'
     '    Delete "$SMPROGRAMS\\章八爪\\章八爪.lnk"\n    Delete "$SMPROGRAMS\\章八爪\\卸载章八爪.lnk"\n    RMDir "$SMPROGRAMS\\章八爪"\n', 1),
]

# 补丁后不允许再出现的可见身份（文件 → 正则）
FORBIDDEN = {
    "backend/internal/desktopstorage/location.go": [r'Join\(root, "BeefTV"\)'],
    "backend/cmd/desktop/storage.go": [r'"BeefTV"', r"选择 BeefTV"],
    "backend/internal/desktopupdate/config.go": [r'windowsExeName\s*=\s*"BeefTV\.exe"'],
    "backend/internal/desktopupdate/engine.go": [r'"BeefTV", "updates"'],
    "backend/cmd/desktop/wails.json": [r'"(name|outputfilename)": "BeefTV"'],
    "scripts/build-beeftv-windows-release.ps1": [r'Join-Path \$binDir "BeefTV\.exe"'],
    "scripts/package-desktop-installer.py": [r"'BeefTV\.exe' if platform == 'windows-amd64'"],
    "scripts/installers/windows.nsi": [r"Programs\\BeefTV", r"\$INSTDIR\\BeefTV\.exe", r"LOCALAPPDATA\\BeefTV", r'CreateShortcut "\$SMPROGRAMS\\BeefTV',
                                       r'StrCpy \$1 "\$1\\BeefTV"', r'"LegalCopyright" "BeefTV"'],
}
REQUIRED = {
    "backend/cmd/desktop/main.go": ["zbzid.WebviewDir()"],
    "backend/internal/desktopstorage/location.go": ["zbzid.DataRoot(root)"],
    "scripts/installers/windows.nsi": ['$INSTDIR\\ZhangBaZhua.exe', NEW_KEY, 'DeleteRegKey HKCU ' + OLD_KEY],
}


def read(p):
    raw = (ROOT / p).read_bytes()
    bom = raw.startswith(b"\xef\xbb\xbf")
    return raw.decode("utf-8-sig"), bom


def write(p, text, bom):
    data = text.encode("utf-8")
    (ROOT / p).write_bytes((b"\xef\xbb\xbf" if bom else b"") + data)


def apply():
    errors, done = [], 0
    cache = {}
    for path, old, new, expect in PATCHES:
        if path not in cache:
            cache[path] = list(read(path))
        text = cache[path][0]
        if new in text and (old not in new or text.count(old) == text.count(new)):
            continue  # 幂等：已打过
        n = text.count(old)
        if n == 0 or (expect is not None and n != expect):
            errors.append(f"{path}：期望命中 {expect or '≥1'} 次，实际 {n} 次 —— {old[:60]!r}")
            continue
        cache[path][0] = text.replace(old, new)
        done += n
    for path, (text, bom) in cache.items():
        write(path, text, bom)
    return errors, done


def audit():
    found = []
    for path, pats in FORBIDDEN.items():
        text, _ = read(path)
        for line_no, line in enumerate(text.splitlines(), 1):
            s = line.strip()
            if s.startswith(("#", "//", ";")):
                continue
            for pat in pats:
                if re.search(pat, line):
                    found.append(f"{path}:{line_no}: {s[:100]}")
        if path.endswith(".nsi"):
            for line_no, line in enumerate(text.splitlines(), 1):
                if line.startswith("LangString") and "BeefTV" in line:
                    found.append(f"{path}:{line_no}: 安装器文案仍含 BeefTV")
    for path, needles in REQUIRED.items():
        text, _ = read(path)
        for nd in needles:
            if nd not in text:
                found.append(f"{path}: 缺少 {nd}")
    return found


def patch_langstrings():
    """安装器提示文案：中文写章八爪，英文写 ZhangBaZhua。"""
    p = "scripts/installers/windows.nsi"
    text, bom = read(p)
    out = []
    for line in text.splitlines(keepends=True):
        if line.startswith("LangString") and "BeefTV" in line:
            line = line.replace("BeefTV", CN if "LANG_SIMPCHINESE" in line else NEW)
            line = re.sub(rf"{CN} (?=[\u4e00-\u9fff])", CN, line)
            line = re.sub(rf"(?<=[\u4e00-\u9fff]) {CN}", CN, line)
        out.append(line)
    write(p, "".join(out), bom)


def main():
    if "--check" in sys.argv:
        found = audit()
        if found:
            print("身份审计未通过，仍有可见的 BeefTV：", *found, sep="\n  ")
            sys.exit(1)
        print("身份审计通过：可见触点已全部为 章八爪 / ZhangBaZhua（C 类内部标识按约定保留）。")
        return
    errors, done = apply()
    patch_langstrings()
    if errors:
        print("身份补丁失败（上游可能改了这些代码，需要人工更新 identity.py）：", *errors, sep="\n  ")
        sys.exit(1)
    print(f"身份补丁完成：{done} 处替换。")


if __name__ == "__main__":
    main()
