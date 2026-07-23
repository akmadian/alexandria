import type { Meta, StoryObj } from "@storybook/react-vite";
import { byType, cssVar, Mono, Page, Section, type Token } from "./specimens";

const SAMPLE = "Alexandria — the quiet library";
const PANGRAM = "The five boxing wizards jump quickly. 0123456789";

// role slug (matches the emitted .alx-type-<role> class and the metric var names)
const roleSlug = (token: Token) => token.path.replace("type-role.", "");
const metric = (token: Token, key: string) => token.variables?.[`--alx-type-role-${roleSlug(token)}-${key}`] ?? "";
const fontSize = (token: Token) => parseFloat(metric(token, "font-size")) || 0;

// Largest → smallest reads as a scale ramp.
const typeRoles = byType("typography").slice().sort((a, b) => fontSize(b) - fontSize(a));

function TypeSpecimen({ token }: { token: Token }) {
    const slug = roleSlug(token);
    const family = metric(token, "font-family").split(",")[0];
    return (
        <div style={{ display: "grid", gridTemplateColumns: "1fr 240px", gap: "var(--alx-space-6)", alignItems: "baseline", padding: "var(--alx-space-3) 0", borderBottom: "1px solid var(--alx-ink-hairline)" }}>
            <span className={`alx-type-${slug}`}>{SAMPLE}</span>
            <div className="alx-type-caption" style={{ color: "var(--alx-ink-3)", lineHeight: 1.5 }}>
                <Mono>.alx-type-{slug}</Mono>
                <div>
                    {family} · {metric(token, "font-size")}/{metric(token, "line-height")} · {metric(token, "font-weight")}
                    {metric(token, "letter-spacing") && ` · ${metric(token, "letter-spacing")}`}
                </div>
                {token.role && <div style={{ color: "var(--alx-ink-4)" }}>{token.role}</div>}
            </div>
        </div>
    );
}

const FAMILIES = [
    { path: "font.sans", label: "Sans — Geist" },
    { path: "font.mono", label: "Mono — Geist Mono" },
    { path: "font.joy-serif", label: "Serif — Instrument Serif" },
    { path: "font.joy-pixel", label: "Pixel — Geist Pixel" },
];

const WEIGHTS = [
    { path: "weight.regular", label: "Regular" },
    { path: "weight.medium", label: "Medium" },
    { path: "weight.semibold", label: "Semibold" },
];

const meta: Meta = {
    title: "Design System/Typography",
    parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

export const Typography: Story = {
    render: () => (
        <Page title="Typography" intro="Type roles are units — size, line-height, weight and tracking move together (setting font-size alone is a defect). Each specimen is rendered in the live role class.">
            <Section title="Type roles" hint={`${typeRoles.length} roles, largest → smallest`}>
                {typeRoles.map((token) => (
                    <TypeSpecimen key={token.path} token={token} />
                ))}
            </Section>
            <Section title="Font families">
                <div style={{ display: "grid", gap: "var(--alx-space-4)" }}>
                    {FAMILIES.map((entry) => (
                        <div key={entry.path}>
                            <div className="alx-type-caption" style={{ color: "var(--alx-ink-3)", marginBottom: "var(--alx-space-1)" }}>
                                {entry.label} · <Mono>{cssVar(entry.path)}</Mono>
                            </div>
                            <div style={{ fontFamily: `var(${cssVar(entry.path)})`, fontSize: 22 }}>{PANGRAM}</div>
                        </div>
                    ))}
                </div>
            </Section>
            <Section title="Weights">
                <div style={{ display: "flex", gap: "var(--alx-space-8)", flexWrap: "wrap" }}>
                    {WEIGHTS.map((entry) => (
                        <div key={entry.path}>
                            <span style={{ fontWeight: `var(${cssVar(entry.path)})` as unknown as number, fontSize: 28 }}>Aa</span>
                            <div className="alx-type-caption" style={{ color: "var(--alx-ink-3)" }}>
                                {entry.label}
                            </div>
                        </div>
                    ))}
                </div>
            </Section>
        </Page>
    ),
};
