import type { Meta, StoryObj } from "@storybook/react-vite";
import { cssVar, Mono, Page, rawValue, Section, TOKENS, type Token } from "./specimens";

// The space scale only — control / icon / row sizing lives on the dedicated
// "Control Sizes" page (the D33 proportional ladder).
const px = (token: Token) => parseFloat(rawValue(token)) || 0;
const spaceScale = TOKENS.filter((token) => token.path.startsWith("space.")).sort((a, b) => px(a) - px(b));

function ScaleRow({ token }: { token: Token }) {
    return (
        <div style={{ display: "flex", gap: "var(--alx-space-4)", alignItems: "center" }}>
            <Mono>{cssVar(token.path)}</Mono>
            <span className="alx-type-caption" style={{ width: 48, color: "var(--alx-ink-3)" }}>
                {rawValue(token)}
            </span>
            <span style={{ height: 16, width: `var(${cssVar(token.path)})`, background: "var(--alx-accent)", borderRadius: 2 }} />
        </div>
    );
}

const meta: Meta = {
    title: "Design System/Spacing",
    parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

export const Spacing: Story = {
    render: () => (
        <Page title="Spacing" intro="Spacing is quantum multiples via --alx-space-N — bars drawn at true width.">
            <Section title="Space scale" hint={`${spaceScale.length} steps`}>
                <div style={{ display: "grid", gap: "var(--alx-space-2)" }}>
                    {spaceScale.map((token) => (
                        <ScaleRow key={token.path} token={token} />
                    ))}
                </div>
            </Section>
        </Page>
    ),
};
