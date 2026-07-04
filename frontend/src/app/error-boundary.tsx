// The one class component (React still requires a class for componentDidCatch).
// ~30 lines beats a dependency. Regional boundaries wrap each shell pane so a
// crash degrades that pane, not the app; the root boundary catches the rest.

import { Component, type ErrorInfo, type ReactNode } from "react";
import { log } from "@/lib/logger";

interface Props {
    /** Rendered on crash. `retry` remounts the children via a key bump. */
    fallback: (error: Error, retry: () => void) => ReactNode;
    children: ReactNode;
}

interface State {
    error: Error | null;
    attempt: number;
}

export class ErrorBoundary extends Component<Props, State> {
    state: State = { error: null, attempt: 0 };

    static getDerivedStateFromError(error: Error): Partial<State> {
        return { error };
    }

    componentDidCatch(error: Error, info: ErrorInfo): void {
        log.error("react boundary caught", { message: error.message, stack: error.stack, componentStack: info.componentStack });
    }

    retry = (): void => {
        this.setState((s) => ({ error: null, attempt: s.attempt + 1 }));
    };

    render(): ReactNode {
        if (this.state.error) return this.props.fallback(this.state.error, this.retry);
        return <span key={this.state.attempt} style={{ display: "contents" }}>{this.props.children}</span>;
    }
}
