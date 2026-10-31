export type AppRoute = {
  title: string;
  element: string;
  admin: boolean;
  load: () => Promise<unknown>;
};

export const appRoutes: Readonly<Record<string, AppRoute>> = {
  '/dashboard': { title: '数据看板', element: 'sp-dashboard-page', admin: true, load: () => import('../dashboard') },
  '/orders': { title: '订单', element: 'sp-orders-page', admin: true, load: () => import('../orders') },
  '/notifications': { title: '使用日志', element: 'sp-notifications-page', admin: true, load: () => import('../notifications') },
  '/groups': { title: 'API令牌', element: 'sp-groups-page', admin: true, load: () => import('../groups') },
  '/sandbox': { title: '沙箱', element: 'sp-sandbox-page', admin: true, load: () => import('../sandbox') },
  '/developer': { title: '开发者接入', element: 'sp-developer-page', admin: true, load: () => import('../developer') },
  '/operations': { title: '运行维护', element: 'sp-operations-page', admin: true, load: () => import('../operations') },
  '/team': { title: '团队与账号', element: 'sp-team-page', admin: true, load: () => import('../team') },
  '/login': { title: '登录', element: 'sp-login-page', admin: false, load: () => import('../login') },
  '/cashier': { title: '收银台', element: 'sp-cashier', admin: false, load: () => import('../cashier') },
};

export function isAppRoute(pathname: string): boolean {
  return pathname === '/' || Object.hasOwn(appRoutes, pathname);
}

export function currentRoute(): AppRoute | undefined {
  return appRoutes[location.pathname];
}

type RouteChange = { kind: 'initial' | 'push' | 'replace' | 'pop'; scroll?: [number, number] };
type RouteListener = (change: RouteChange) => void;
const listeners = new Set<RouteListener>();
const scrollKey = 'mintweftScroll';
let previousScrollRestoration: ScrollRestoration = 'auto';

function rememberScroll() {
  const state = history.state && typeof history.state === 'object' ? history.state : {};
  history.replaceState({ ...state, [scrollKey]: [scrollX, scrollY] }, '', location.href);
}

function emit(change: RouteChange) {
  for (const listener of listeners) listener(change);
}

export function navigate(target: string | URL, options: { replace?: boolean } = {}) {
  const url = new URL(target, location.href);
  if (!['http:', 'https:'].includes(url.protocol)) return;
  if (url.origin !== location.origin || !isAppRoute(url.pathname) || url.username || url.password || !listeners.size) {
    if (options.replace) location.replace(url.href);
    else location.assign(url.href);
    return;
  }
  if (url.href === location.href) return;
  rememberScroll();
  const state = { [scrollKey]: [0, 0] };
  if (options.replace) history.replaceState(state, '', url.href);
  else history.pushState(state, '', url.href);
  emit({ kind: options.replace ? 'replace' : 'push', scroll: [0, 0] });
}

function pop(event: PopStateEvent) {
  const coordinates: unknown = event.state?.[scrollKey];
  const scroll: [number, number] | undefined = Array.isArray(coordinates) && coordinates.length === 2
    && coordinates.every(value => typeof value === 'number' && Number.isFinite(value))
    ? [coordinates[0], coordinates[1]] : undefined;
  emit({ kind: 'pop', scroll });
}

function click(event: MouseEvent) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return;
  const anchor = event.composedPath().find(node => node instanceof HTMLAnchorElement);
  if (!(anchor instanceof HTMLAnchorElement) || !anchor.hasAttribute('href') || anchor.hasAttribute('download')
    || (anchor.target && anchor.target.toLowerCase() !== '_self')
    || anchor.relList.contains('external')) return;
  const url = new URL(anchor.href, location.href);
  if (url.origin !== location.origin || !['http:', 'https:'].includes(url.protocol) || !isAppRoute(url.pathname) || url.username || url.password) return;
  event.preventDefault();
  navigate(url);
}

/** One document-level listener also receives composed clicks from Lit shadow roots. */
export function subscribeRoutes(listener: RouteListener): () => void {
  if (!listeners.size) {
    previousScrollRestoration = history.scrollRestoration;
    history.scrollRestoration = 'manual';
    document.addEventListener('click', click);
    window.addEventListener('popstate', pop);
  }
  listeners.add(listener);
  listener({ kind: 'initial' });
  return () => {
    listeners.delete(listener);
    if (!listeners.size) {
      document.removeEventListener('click', click);
      window.removeEventListener('popstate', pop);
      history.scrollRestoration = previousScrollRestoration;
    }
  };
}
