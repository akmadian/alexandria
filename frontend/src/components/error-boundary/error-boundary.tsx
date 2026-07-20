// PaneErrorBoundary — the per-pane crash wall (frontend-architecture: browser /
// main region / inspector fail independently, reload via key bump). Catches
// RENDER errors only; async failures route through ApiError and each
// consumer's explicit error state. Domain-blind chrome, no design-library
// matrix: its only visual state is the fallback row.

import { Component, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/button/button";
import { cx } from "@/lib/cx";
import { log } from "@/lib/logger";
import styles from "./error-boundary.module.css";

interface CatcherProps {
    fallback: (reset: () => void) => ReactNode;
    children: ReactNode;
}

interface CatcherState {
    failed: boolean;
    /** Bumped on reset so the subtree remounts clean instead of re-throwing. */
    epoch: number;
}

class Catcher extends Component<CatcherProps, CatcherState> {
    state: CatcherState = { failed: false, epoch: 0 };

    static getDerivedStateFromError(): Partial<CatcherState> {
        return { failed: true };
    }

    componentDidCatch(error: Error): void {
        log.error("pane crashed", { error: String(error) });
    }

    reset = (): void => {
        this.setState((previous) => ({ failed: false, epoch: previous.epoch + 1 }));
    };

    render(): ReactNode {
        if (this.state.failed) return this.props.fallback(this.reset);
        return (
            <div key={this.state.epoch} className={styles.host}>
                {this.props.children}
            </div>
        );
    }
}

export function PaneErrorBoundary({ children }: { children: ReactNode }) {
    const { t } = useTranslation();
    return (
        <Catcher
            fallback={(reset) => (
                <div className={styles.fallback}>
                    <span className={cx(styles.message, "alx-type-label")}>{t("errors.panelCrashed")}</span>
                    <Button rung="outline" onPress={reset}>
                        {t("errors.reloadPanel")}
                    </Button>
                </div>
            )}
        >
            {children}
        </Catcher>
    );
}
