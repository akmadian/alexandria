import type { Meta, StoryObj } from "@storybook/react-vite";
import { ControlRow } from "@/components/control-row/control-row";
import { Row, type RowIntent } from "@/components/row/row";
import { Switch } from "@/components/switch/switch";
import { PanelSection } from "./panel-section";

const INTENTS: RowIntent[] = ["control", "list", "text"];

const meta = {
    title: "Primitives/PanelSection",
    component: PanelSection,
    // children (the section rows) is supplied by each story's render.
    args: { head: "Metadata", intent: "control", defaultExpanded: true, children: null },
    argTypes: {
        head: { control: "text" },
        intent: { control: "inline-radio", options: INTENTS },
        defaultExpanded: { control: "boolean" },
    },
} satisfies Meta<typeof PanelSection>;

export default meta;

type Story = StoryObj<typeof meta>;

// Interactive: an expanded section whose rows inherit the section's chosen intent (§8).
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
            <PanelSection {...args}>
                <Row label="Camera" value="Leica M11" />
                <Row label="Lens" value="35mm f/1.4" />
                <Row label="ISO" value="200" />
            </PanelSection>
        </div>
    ),
};

// The section owns the density: it chooses the intent for every row inside via context.
export const Expanded: Story = {
    render: () => (
        <div
            style={{
                width: 320,
                background: "var(--alx-surface-panel)",
                border: "var(--alx-stroke-hairline) solid var(--alx-ink-hairline)",
                borderRadius: "var(--alx-radius-docked)",
            }}
        >
            <PanelSection head="Import" intent="control">
                <ControlRow label="Sync sidecar" size="sm">
                    <Switch size="sm" aria-label="Sync sidecar" defaultSelected />
                </ControlRow>
                <ControlRow label="Write XMP" size="sm">
                    <Switch size="sm" aria-label="Write XMP" />
                </ControlRow>
                <Row label="Watched folder" value="/Volumes/Shoots" />
            </PanelSection>
        </div>
    ),
};

// Collapsed: the head + chevron with the panel folded away (defaultExpanded=false).
export const Collapsed: Story = {
    render: () => (
        <div
            style={{
                width: 320,
                background: "var(--alx-surface-panel)",
                border: "var(--alx-stroke-hairline) solid var(--alx-ink-hairline)",
                borderRadius: "var(--alx-radius-docked)",
            }}
        >
            <PanelSection head="Metadata" defaultExpanded={false}>
                <Row label="Camera" value="Leica M11" />
                <Row label="Lens" value="35mm f/1.4" />
            </PanelSection>
        </div>
    ),
};
