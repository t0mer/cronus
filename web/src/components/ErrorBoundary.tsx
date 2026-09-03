import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertTriangle, RotateCcw } from "lucide-react";
import { Button, Card } from "./ui";

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

// ErrorBoundary catches render-time errors in the page tree and shows a
// fallback instead of unmounting the whole app (which would blank the screen).
// Wrap it with a key that changes on navigation so switching tabs recovers.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Surface the error for debugging; the fallback keeps the app usable.
    console.error("Unhandled UI error:", error, info.componentStack);
  }

  handleReset = () => this.setState({ error: null });

  render() {
    if (this.state.error) {
      return (
        <Card className="mx-auto max-w-lg p-8 text-center">
          <div className="mx-auto mb-3 flex h-10 w-10 items-center justify-center rounded-full bg-danger/15 text-danger">
            <AlertTriangle className="h-5 w-5" />
          </div>
          <h2 className="font-display text-lg font-semibold">Something went wrong on this page</h2>
          <p className="mx-auto mt-1 max-w-sm text-sm text-muted">
            An unexpected error interrupted this view. Try again, or switch to another tab.
          </p>
          <p className="mx-auto mt-3 max-w-sm break-words rounded-md bg-surface-2 px-3 py-2 text-left font-mono text-xs text-muted">
            {this.state.error.message}
          </p>
          <div className="mt-4 flex justify-center gap-2">
            <Button variant="outline" onClick={this.handleReset}>
              <RotateCcw className="h-4 w-4" /> Try again
            </Button>
            <Button onClick={() => window.location.reload()}>Reload page</Button>
          </div>
        </Card>
      );
    }
    return this.props.children;
  }
}
