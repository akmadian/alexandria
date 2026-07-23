import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@/components/button/button";
import { Switch } from "@/components/switch/switch";
import { ControlRow, type ControlRowSize } from "./control-row";

const SIZES: ControlRowSize[] = ["xs", "sm", "md", "lg"];

const meta = {
    title: "Primitives/ControlRow",
    component: ControlRow,
    // children (the hosted control) is supplied by each story's render.
    args: { label: "Sync sidecar", size: "md", children: null },
    argTypes: {
        size: { control: "inline-radio", options: SIZES },
        label: { control: "text" },
    },
} satisfies Meta<typeof ControlRow>;

export default meta;

type Story = StoryObj<typeof meta>;

// Interactive: drive label/size from the Controls panel; the row hosts a real control.
export const Playground: Story = {
    render: (args) => (
        <div style={{ width: 320 }}>
            <ControlRow {...args}>
                <Switch size={args.size} aria-label={typeof args.label === "string" ? args.label : "toggle"} />
            </ControlRow>
        </div>
    ),
};

// The four ladder heights (16/20/24/28), each a control-row hosting a sample control at
// its matching size — the row owns only its height; the control brings its own size (D33).
export const Matrix: Story = {
    render: () => (
        <div style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-4)", width: 320 }}>
            {SIZES.map((size) => (
                <div key={size} style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-1)" }}>
                    <span className="alx-type-caption">{size}</span>
                    <div
                        style={{
                            background: "var(--alx-surface-panel)",
                            border: "var(--alx-stroke-hairline) solid var(--alx-ink-hairline)",
                            borderRadius: "var(--alx-radius-docked)",
                        }}
                    >
                        <ControlRow label="Sync sidecar" size={size}>
                            <Switch size={size} aria-label="Sync sidecar" />
                        </ControlRow>
                    </div>
                </div>
            ))}
            <span className="alx-type-caption">md — hosting a button instead</span>
            <div
                style={{
                    background: "var(--alx-surface-panel)",
                    border: "var(--alx-stroke-hairline) solid var(--alx-ink-hairline)",
                    borderRadius: "var(--alx-radius-docked)",
                }}
            >
                <ControlRow label="Sidecar" size="md">
                    <Button size="sm">Re-sync</Button>
                </ControlRow>
            </div>
        </div>
    ),
};
