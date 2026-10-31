import type { LitElement, ReactiveController } from 'lit';

/** Request ownership follows the component's connection lifecycle. */
export class RequestController implements ReactiveController {
  private readonly requests = new Map<string, AbortController>();
  private readonly hide = (event: PageTransitionEvent) => {
    if (!event.persisted) this.abort();
  };
  private readonly show = (event: PageTransitionEvent) => {
    if (event.persisted && this.host.isConnected) this.onResume?.();
  };

  constructor(
    private readonly host: LitElement,
    private readonly onResume?: () => void,
  ) {
    host.addController(this);
  }

  hostConnected() {
    window.addEventListener('pagehide', this.hide);
    window.addEventListener('pageshow', this.show);
  }

  hostDisconnected() {
    this.abort();
    window.removeEventListener('pagehide', this.hide);
    window.removeEventListener('pageshow', this.show);
  }

  next(channel = 'default'): AbortSignal {
    this.requests.get(channel)?.abort();
    const request = new AbortController();
    this.requests.set(channel, request);
    if (!this.host.isConnected) request.abort();
    return request.signal;
  }

  abort(channel?: string) {
    if (channel !== undefined) {
      this.requests.get(channel)?.abort();
      this.requests.delete(channel);
      return;
    }
    for (const request of this.requests.values()) request.abort();
    this.requests.clear();
  }

  release(channel: string, signal: AbortSignal) {
    if (this.requests.get(channel)?.signal === signal) this.requests.delete(channel);
  }
}
