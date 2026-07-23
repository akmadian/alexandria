// Shared toolkit for the Design System specimen pages. Everything reads the
// compiler's own tokens-reference.json (D31), so specimens and product cannot
// drift — there is one source of truth. Not a *.stories file, so Storybook does
// not treat it as a story; it is the presentational vocabulary the pages compose.
import type { ReactNode } from "react";
import reference from "@/styles/tokens-reference.json";

export interface Token {
    path: string;
    type: string;
    varying: boolean;
    role?: string;
    // Fixed tokens carry a single css value; varying tokens carry one per theme;
    // composite tokens (type roles) carry a `variables` map and no `css`.
    css?: string | Record<string, string>;
    variables?: Record<string, string | undefined>;
}

export const TOKENS: Token[] = reference.tokens;
export const THEMES: string[] = reference.themes;
export const DEFAULT_THEME: string = reference.defaultTheme;

/** Token path → emitted CSS custom property (the strict path mirror, dots→hyphens). */
export const cssVar = (path: string) => `--alx-${path.split(".").join("-")}`;
export const byType = (type: string) => TOKENS.filter((token) => token.type === type);
export const find = (path: string) => TOKENS.find((token) => token.path === path);
export const has = (path: string) => TOKENS.some((token) => token.path === path);

/** The source value for the default theme — reference text only; the live var is truth. */
export const rawValue = (token?: Token): string => {
    if (!token) return "";
    if (typeof token.css === "string") return token.css;
    if (token.css) return token.css[DEFAULT_THEME] ?? "";
    return "";
};

/** First clause of a role blurb — the long constitution citations are for the docs, not the chip. */
export const shortRole = (role?: string) => (role ?? "").split(/[.;(]/)[0].trim();

// ── Layout primitives ──────────────────────────────────────────────────────

export function Page({ title, intro, children }: { title: string; intro?: ReactNode; children: ReactNode }) {
    return (
        <div style={{ maxWidth: 1120, color: "var(--alx-ink-1)" }}>
            <h1 className="alx-type-title" style={{ marginBottom: "var(--alx-space-2)" }}>
                {title}
            </h1>
            {intro && (
                <p className="alx-type-caption" style={{ color: "var(--alx-ink-3)", marginBottom: "var(--alx-space-6)", maxWidth: 640 }}>
                    {intro}
                </p>
            )}
            {children}
        </div>
    );
}

export function Section({ title, hint, children }: { title: string; hint?: string; children: ReactNode }) {
    return (
        <section style={{ marginBottom: "var(--alx-space-8)" }}>
            <h2 className="alx-type-label" style={{ textTransform: "uppercase", letterSpacing: "0.06em", color: "var(--alx-ink-3)", marginBottom: "var(--alx-space-3)" }}>
                {title}
                {hint && (
                    <span className="alx-type-caption" style={{ marginLeft: "var(--alx-space-2)", textTransform: "none", letterSpacing: 0, color: "var(--alx-ink-4)" }}>
                        {hint}
                    </span>
                )}
            </h2>
            {children}
        </section>
    );
}

/** Responsive card grid used by the semantic-color and reference sections. */
export function CardGrid({ min = 240, children }: { min?: number; children: ReactNode }) {
    return <div style={{ display: "grid", gridTemplateColumns: `repeat(auto-fill, minmax(${min}px, 1fr))`, gap: "var(--alx-space-4)" }}>{children}</div>;
}

export function Mono({ children }: { children: ReactNode }) {
    return (
        <code className="alx-type-data-sm" style={{ color: "var(--alx-ink-2)" }}>
            {children}
        </code>
    );
}

/** A color chip painting the live variable — reflows when the toolbar theme changes. */
export function Swatch({ path, size = 44, radius = "var(--alx-radius-control, 6px)" }: { path: string; size?: number; radius?: string }) {
    return (
        <span
            title={cssVar(path)}
            style={{
                display: "block",
                width: size,
                height: size,
                flex: "none",
                borderRadius: radius,
                background: `var(${cssVar(path)})`,
                boxShadow: "inset 0 0 0 1px var(--alx-ink-hairline, rgba(128,128,128,.3))",
            }}
        />
    );
}

/** Swatch + name + short role — the semantic-color unit. */
export function SwatchCard({ token }: { token: Token }) {
    return (
        <div style={{ display: "flex", gap: "var(--alx-space-3)", alignItems: "center", minWidth: 0 }}>
            <Swatch path={token.path} />
            <div style={{ minWidth: 0 }}>
                <Mono>{cssVar(token.path)}</Mono>
                {token.role && (
                    <span
                        className="alx-type-caption"
                        style={{ display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden", color: "var(--alx-ink-3)" }}
                    >
                        {shortRole(token.role)}
                    </span>
                )}
            </div>
        </div>
    );
}
