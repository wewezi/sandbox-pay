/**
 * theme.js 挂在 window 上的全局。
 *
 * 它必须是 classic script（同步跑在 <head> 里，否则切暗色会闪白底 ——
 * 原因见 theme.js 头部注释），所以没法 import，只能靠 window 通信。
 * 这里把那个契约写成类型，app.ts 的主题组件用得到。
 */

/** 主题三态。system = 跟随系统，实际渲染值要用 effective() 解析。 */
type ThemeMode = 'light' | 'dark' | 'system';

/** effective() 解析后的结果，只会是这两个。 */
type EffectiveTheme = 'light' | 'dark';

interface ThemeApi {
  get(): ThemeMode;
  /** 把 system 解析成当前系统实际的亮/暗 */
  effective(mode?: ThemeMode): EffectiveTheme;
  set(mode: ThemeMode): void;
  /** 亮 → 暗 → 跟随系统，循环，返回切换后的值 */
  cycle(): ThemeMode;
  /** 注册变更回调，返回取消函数 */
  onChange(
    fn: (mode: ThemeMode, effective: EffectiveTheme) => void,
  ): () => void;
}

interface Window {
  /** theme.js 加载失败时是 undefined，调用方要判空 */
  __theme?: ThemeApi;
}
