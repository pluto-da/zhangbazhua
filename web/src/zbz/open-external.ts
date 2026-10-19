// 章八爪模块 · 用系统浏览器打开外部链接 · 作者 长鱼茶汤茗
type WailsRuntime = { BrowserOpenURL?: (url: string) => void };

export function openExternal(url: string) {
    const runtime = (window as unknown as { runtime?: WailsRuntime }).runtime;
    if (runtime?.BrowserOpenURL) {
        runtime.BrowserOpenURL(url);
        return;
    }
    window.open(url, "_blank", "noopener,noreferrer");
}
