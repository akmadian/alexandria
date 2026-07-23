import type { Meta, StoryObj } from "@storybook/react-vite";
import { Row, type RowIntent } from "./row";

const INTENTS: RowIntent[] = ["control", "list", "text"];

// What each intent is FOR — the intent binds height + the permitted type roles (§8/§12).
const INTENT_NOTES: Record<RowIntent, string> = {
    control: "28px — the control-height metadata row",
    list: "24px — a scannable list line (value-role label)",
    text: "16px — the densest read-only text row",
};

const meta = {
    title: "Primitives/Row",
    component: Row,
    args: { intent: "control", label: "Aperture", value: "f/2.8" },
    argTypes: {
        intent: { control: "inline-radio", options: INTENTS },
        label: { control: "text" },
        value: { control: "text" },
    },
} satisfies Meta<typeof Row>;

export default meta;

type Story = StoryObj<typeof meta>;

// Interactive: drive intent/label/value from the Controls panel.
export const Playground: Story = {
    render: (args) => (
        <div style={{ width: 280 }}>
            <Row {...args} />
        </div>
    ),
};

// The three intents, each a real row — they bind different heights and type roles.
export const Matrix: Story = {
    render: () => (
        <div style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-6)", width: 280 }}>
            {INTENTS.map((intent) => (
                <div key={intent} style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-1)" }}>
                    <span className="alx-type-caption">
                        {intent} — {INTENT_NOTES[intent]}
                    </span>
                    <div
                        style={{
                            background: "var(--alx-surface-panel)",
                            border: "var(--alx-stroke-hairline) solid var(--alx-ink-hairline)",
                            borderRadius: "var(--alx-radius-docked)",
                        }}
                    >
                        <Row intent={intent} label="Aperture" value="f/2.8" />
                    </div>
                </div>
            ))}
        </div>
    ),
};
