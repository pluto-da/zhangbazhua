#!/usr/bin/env python3
"""章八爪模块 · 作者 长鱼茶汤茗
构建时品牌替换：把用户可见的产品自称「BeefTV」换成「章八爪」。
- A 类（产品自称）替换；B 类（官方算力服务 beeftv.app）与 C 类（标识符、路径、文件名、URL、整串字面量）保留。
- 幂等；在 CI 编译前运行，不提交到 custom 分支，所以和上游合并永远不冲突。
用法：python scripts/zbz/rebrand.py [--check]   --check 只审计不改，发现 A 类遗留返回 1。
"""
import pathlib, re, sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
NAME = "章八爪"

# 扫描范围
GLOBS = [
    ("web/src", ("*.ts", "*.tsx")),
    ("web/public/welcome", ("*.html",)),
    ("web", ("index.html",)),
    ("backend", ("*.go", "*.md")),
    ("backend/internal/skills/seed", ("skills.json",)),
    ("agent-host", ("*.mjs",)),
    ("scripts/installers", ("windows.nsi",)),
]
SKIP_PARTS = ("node_modules", "__tests__", "/testdata/", "/dist/", "cmd/update-release", "web/src/zbz/", "internal/zbz")  # 自有模块已是章八爪品牌，且含对上游的署名，不替换

def skip(p: pathlib.Path) -> bool:
    s = p.as_posix()
    n = p.name
    return (any(x in s for x in SKIP_PARTS) or n.endswith("_test.go") or ".test." in n or ".spec." in n
            or "test-runner" in n or n.endswith(".d.ts"))

# 先做的精确替换（A 类，措辞需要调整的）
EXACT = [
    ("BeefTV 团队", f"{NAME}（基于 BeefTV 团队）"),
]
# B 类与出处说明：整段保护，不替换
PROTECT = [
    r"连接\s*BeefTV(?!\s*工作区)", r"BeefTV\s*(?:连接|服务|账户|账号|官方|还没连接|算力)", r"based on BeefTV", r"基于\s*BeefTV",
    r"打开\s*BeefTV\s*账户", r"BeefTV\s*Contributors",
]
# 独立词 BeefTV：前后不能是标识符、路径、文件名、URL 的一部分
TOKEN = re.compile(r"(?<![A-Za-z0-9_./\\\-$%])BeefTV(?![A-Za-z0-9_\-/]|\.(?:exe|app|lnk|update|Installation)|\\)")
EXACT_LITERAL = re.compile(r"""(["'`])BeefTV\1""")
CJK = r"[\u3400-\u9fff\uff00-\uffef\u3000-\u303f]"

def transform(text: str):
    count = 0
    for a, b in EXACT:
        if a in text and b not in text:
            count += text.count(a); text = text.replace(a, b)
    holds = []
    def hold(m):
        holds.append(m.group(0)); return f"\u0000{len(holds)-1}\u0000"
    for pat in PROTECT:
        text = re.sub(pat, hold, text)
    text = EXACT_LITERAL.sub(hold, text)
    text, n = TOKEN.subn(NAME, text)
    count += n
    if n:
        text = re.sub(rf"{NAME} (?={CJK})", NAME, text)
        text = re.sub(rf"(?<={CJK}) {NAME}", NAME, text)
    text = re.sub("\u0000(\\d+)\u0000", lambda m: holds[int(m.group(1))], text)
    return text, count

def files():
    for base, pats in GLOBS:
        d = ROOT / base
        for pat in pats:
            it = d.glob(pat) if base == "web" else d.rglob(pat)
            for p in it:
                if p.is_file() and not skip(p):
                    yield p

def main():
    check = "--check" in sys.argv
    total, changed = 0, []
    for p in sorted(set(files())):
        src = p.read_text(encoding="utf-8")
        out, n = transform(src)
        if n:
            total += n; changed.append((p.relative_to(ROOT).as_posix(), n))
            if not check:
                p.write_text(out, encoding="utf-8")
    label = "待替换（A 类遗留）" if check else "已替换"
    for f, n in changed:
        print(f"{label} {n:3d}  {f}")
    print(f"合计 {total} 处，{len(changed)} 个文件")
    if check and total:
        sys.exit(1)

if __name__ == "__main__":
    main()
