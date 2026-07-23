import type { Meta, StoryObj } from "@storybook/react-vite";
import { ToggleButton, type ToggleButtonSize } from "./toggle-button";

const SIZES: ToggleButtonSize[] = ["xs", "sm", "md", "lg"];

// Rows of the matrix: each state is a fixed set of RAC props.
const STATES: { label: string; props: Record<string, boolean> }[] = [
    { label: "rest", props: {} },
    { label: "selected", props: { isSelected: true } },
    { label: "disabled", props: { isDisabled: true } },
];

const meta = {
    title: "Primitives/ToggleButton",
    component: ToggleButton,
    args: {
        children: "Toggle",
        size: "md",
        isSelected: false,
        isDisabled: false,
    },
    argTypes: {
        size: { control: "inline-radio", options: SIZES },
        isSelected: { control: "boolean" },
        isDisabled: { control: "boolean" },
    },
} satisfies Meta<typeof ToggleButton>;

export default meta;

type Story = StoryObj<typeof meta>;

// Interactive: drive size/selected/disabled from the Controls panel.
export const Playground: Story = {};

// The full state × size grid — the specimen an eye-gate wants in one view.
export const Matrix: Story = {
    render: () => (
        <div style={{ display: "grid", gridTemplateColumns: `auto repeat(${SIZES.length}, max-content)`, gap: "var(--alx-space-4)", alignItems: "center" }}>
            <span />
            {SIZES.map((size) => (
                <span key={size} className="alx-type-caption" style={{ textAlign: "center" }}>
                    {size}
                </span>
            ))}
            {STATES.map((state) => (
                <div key={state.label} style={{ display: "contents" }}>
                    <span className="alx-type-caption">{state.label}</span>
                    {SIZES.map((size) => (
                        <ToggleButton key={size} size={size} {...state.props}>
                            Toggle
                        </ToggleButton>
                    ))}
                </div>
            ))}
        </div>
    ),
};
