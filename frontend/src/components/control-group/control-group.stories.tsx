import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@/components/button/button";
import { ControlRow } from "@/components/control-row/control-row";
import { Switch } from "@/components/switch/switch";
import { ControlGroup } from "./control-group";

const meta = {
    title: "Primitives/ControlGroup",
    component: ControlGroup,
    // children is required on the component; each story's render supplies the real
    // subtree, so this placeholder only satisfies the required-prop type.
    args: { labelWidth: "40%", children: null },
    argTypes: {
        labelWidth: { control: "text" },
    },
} satisfies Meta<typeof ControlGroup>;

export default meta;

type Story = StoryObj<typeof meta>;

// A realistic settings-like group: flush ControlRows sharing one aligned label column.
export const Playground: Story = {
    render: (args) => (
        <div
            style={{
                width: 320,
                background: "var(--alx-surface-panel)",
                border: "var(--alx-stroke-hairline) solid var(--alx-ink-hairline)",
                borderRadius: "var(--alx-radius-docked)",
            }}
        >
            <ControlGroup {...args}>
                <ControlRow label="Sync sidecar" size="sm">
                    <Switch size="sm" aria-label="Sync sidecar" defaultSelected />
                </ControlRow>
                <ControlRow label="Write XMP" size="sm">
                    <Switch size="sm" aria-label="Write XMP" />
                </ControlRow>
                <ControlRow label="Auto-stack" size="sm">
                    <Switch size="sm" aria-label="Auto-stack" />
                </ControlRow>
                <ControlRow label="Rebuild" size="sm">
                    <Button size="xs" rung="outline">
                        Run now
                    </Button>
                </ControlRow>
            </ControlGroup>
        </div>
    ),
};

// The same group at two label-column widths — the shared column is set at the group level.
export const LabelWidths: Story = {
    render: () => (
        <div style={{ display: "flex", gap: "var(--alx-space-8)" }}>
            {["30%", "55%"].map((labelWidth) => (
                <div key={labelWidth} style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-1)" }}>
                    <span className="alx-type-caption">labelWidth {labelWidth}</span>
                    <div
                        style={{
                            width: 300,
                            background: "var(--alx-surface-panel)",
                            border: "var(--alx-stroke-hairline) solid var(--alx-ink-hairline)",
                            borderRadius: "var(--alx-radius-docked)",
                        }}
                    >
                        <ControlGroup labelWidth={labelWidth}>
                            <ControlRow label="Sync sidecar" size="sm">
                                <Switch size="sm" aria-label="Sync sidecar" defaultSelected />
                            </ControlRow>
                            <ControlRow label="Write XMP" size="sm">
                                <Switch size="sm" aria-label="Write XMP" />
                            </ControlRow>
                        </ControlGroup>
                    </div>
                </div>
            ))}
        </div>
    ),
};
