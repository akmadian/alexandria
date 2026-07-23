import type { Meta, StoryObj } from "@storybook/react-vite";
import { cssVar, Mono, Page, rawValue, Section, shortRole, TOKENS, type Token } from "./specimens";

// Everything the visual pages already cover; the rest lands here as a plain table.
const SHOWN = (token: Token) =>
    ["color", "typography", "fontFamily", "fontWeight", "shadow", "duration", "cubicBezier"].includes(token.type) ||
    /^(space\.|radius\.|size\.control-|size\.row-|size\.icon)/.test(token.path);

const remaining = TOKENS.filter((token) => !SHOWN(token));
const family = (token: Token) => token.path.split(".")[0];
const families = [...new Set(remaining.map(family))];

function Row({ token }: { token: Token }) {
    return (
        <tr style={{ borderBottom: "1px solid var(--alx-ink-hairline)" }}>
            <td style={{ padding: "var(--alx-space-2) var(--alx-space-4) var(--alx-space-2) 0", verticalAlign: "top" }}>
                <Mono>{cssVar(token.path)}</Mono>
            </td>
            <td className="alx-type-data-sm" style={{ padding: "var(--alx-space-2) var(--alx-space-4) var(--alx-space-2) 0", verticalAlign: "top", whiteSpace: "nowrap", color: "var(--alx-ink-2)" }}>
                {token.type === "gradient" ? <span style={{ display: "inline-block", width: 120, height: 14, borderRadius: 3, background: `var(${cssVar(token.path)})`, verticalAlign: "middle" }} /> : rawValue(token)}
            </td>
            <td className="alx-type-caption" style={{ padding: "var(--alx-space-2) 0", verticalAlign: "top", color: "var(--alx-ink-3)" }}>
                {shortRole(token.role)}
            </td>
        </tr>
    );
}

const meta: Meta = {
    title: "Design System/Reference",
    parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

export const Reference: Story = {
    render: () => (
        <Page title="Reference" intro="The non-visual tokens — z-index order, alphas, focus and stroke widths, panel geometry, and the gradient — grouped by family for lookup.">
            {families.map((name) => (
                <Section key={name} title={name}>
                    <table style={{ width: "100%", borderCollapse: "collapse" }}>
                        <tbody>
                            {remaining
                                .filter((token) => family(token) === name)
                                .map((token) => (
                                    <Row key={token.path} token={token} />
                                ))}
                        </tbody>
                    </table>
                </Section>
            ))}
        </Page>
    ),
};
