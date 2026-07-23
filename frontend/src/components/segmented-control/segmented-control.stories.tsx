import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Key } from "react-aria-components";
import { Segment, SegmentedControl, type SegmentedControlSize } from "./segmented-control";

const SIZES: SegmentedControlSize[] = ["sm", "md", "lg"];

// The three view modes the grid surface offers — a realistic single-select group.
const VIEWS = ["grid", "list", "loupe"] as const;
const VIEW_LABELS: Record<(typeof VIEWS)[number], string> = {
    grid: "Grid",
    list: "List",
    loupe: "Loupe",
};

const meta = {
    title: "Primitives/SegmentedControl",
    component: SegmentedControl,
    // children (the segments) is supplied by each story's render.
    args: { size: "md", isDisabled: false, children: null },
    argTypes: {
        size: { control: "inline-radio", options: SIZES },
        isDisabled: { control: "boolean" },
    },
} satisfies Meta<typeof SegmentedControl>;

export default meta;

type Story = StoryObj<typeof meta>;

// Controlled view-mode switcher — selection is state the consumer owns.
function ViewSwitcher({ size, isDisabled }: { size?: SegmentedControlSize; isDisabled?: boolean }) {
    const [view, setView] = useState<Key>("grid");
    return (
        <SegmentedControl
            aria-label="View mode"
            size={size}
            isDisabled={isDisabled}
            value={view}
            onChange={setView}
        >
            {VIEWS.map((id) => (
                <Segment key={id} id={id}>
                    {VIEW_LABELS[id]}
                </Segment>
            ))}
        </SegmentedControl>
    );
}

// Interactive: drive size/disabled from the Controls panel.
export const Playground: Story = {
    render: (args) => <ViewSwitcher size={args.size} isDisabled={args.isDisabled} />,
};

// Every size rung in one view — the specimen an eye-gate wants.
export const Matrix: Story = {
    render: () => (
        <div
            style={{
                display: "grid",
                gridTemplateColumns: "auto max-content",
                gap: "var(--alx-space-4)",
                alignItems: "center",
            }}
        >
            {SIZES.map((size) => (
                <div key={size} style={{ display: "contents" }}>
                    <span className="alx-type-caption">{size}</span>
                    <SegmentedControl aria-label={`View mode (${size})`} size={size} defaultValue="grid">
                        {VIEWS.map((id) => (
                            <Segment key={id} id={id}>
                                {VIEW_LABELS[id]}
                            </Segment>
                        ))}
                    </SegmentedControl>
                </div>
            ))}
        </div>
    ),
};
