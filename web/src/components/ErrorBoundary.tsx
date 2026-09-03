import { Component, type ErrorInfo, type ReactNode } from "react";

type Props = { children: ReactNode; title?: string };
type State = { error: Error | null };

/** Ловит runtime-ошибки рендера, чтобы не ронять всё SPA в белый экран. */
export default class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error("ErrorBoundary", error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <div style={{ padding: "1.5rem", maxWidth: 560 }}>
          <h2 style={{ marginTop: 0 }}>{this.props.title ?? "Ошибка интерфейса"}</h2>
          <p style={{ color: "#f88" }}>{this.state.error.message || String(this.state.error)}</p>
          <button type="button" onClick={() => this.setState({ error: null })}>
            Попробовать снова
          </button>
          <button type="button" style={{ marginLeft: 8 }} onClick={() => window.location.assign("/")}>
            На дашборд
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
