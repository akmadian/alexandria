import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button, type ButtonRung, type ButtonSize } from "./button";

const RUNGS: ButtonRung[] = ["ghost", "outline", "tint", "fill", "hero"];
const SIZES: ButtonSize[] = ["xs", "sm", "md", "lg"];

const meta = {
    title: "Primitives/Button",
    component: Button,
    args: { children: "Button", rung: "outline", size: "md", isDisabled: false },
    argTypes: {
        rung: { control: "inline-radio", options: RUNGS },
        size: { control: "inline-radio", options: SIZES },
        isDisabled: { control: "boolean" },
    },
} satisfies Meta<typeof Button>;

export default meta;

type Story = StoryObj<typeof meta>;

// Interactive: drive rung/size/disabled from the Controls panel.
export const Playground: Story = {};

// The full prominence × size grid — the specimen an eye-gate wants in one view.
export const Matrix: Story = {
    render: () => (
        <div style={{ display: "grid", gridTemplateColumns: `auto repeat(${SIZES.length}, max-content)`, gap: "var(--alx-space-3)", alignItems: "center" }}>
            <span />
            {SIZES.map((size) => (
                <span key={size} className="alx-type-caption" style={{ textAlign: "center" }}>
                    {size}
                </span>
            ))}
            {RUNGS.map((rung) => (
                <div key={rung} style={{ display: "contents" }}>
                    <span className="alx-type-caption">{rung}</span>
                    {SIZES.map((size) => (
                        <Button key={size} rung={rung} size={size}>
                            Button
                        </Button>
                    ))}
                </div>
            ))}
        </div>
    ),
};
