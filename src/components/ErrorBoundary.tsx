import { Component, type ErrorInfo, type ReactNode } from "react";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("[kafSIEM] Uncaught render error:", error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      if (this.props.fallback) return this.props.fallback;
      return (
        <div className="flex items-center justify-center h-full bg-siem-bg text-siem-text p-8">
          <div className="max-w-md text-center space-y-3">
            <h2 className="text-lg font-semibold text-siem-critical">Something went wrong</h2>
            <p className="text-sm text-siem-muted">{this.state.error.message}</p>
            <button
              className="px-4 py-2 text-sm rounded bg-siem-accent text-white hover:opacity-90"
              onClick={() => window.location.reload()}
            >
              Reload
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
