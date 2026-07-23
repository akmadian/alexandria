import type { Meta, StoryObj } from "@storybook/react-vite";
import { TextField, type TextFieldSize } from "./text-field";

const SIZES: TextFieldSize[] = ["xs", "sm", "md", "lg"];

const meta = {
    title: "Primitives/TextField",
    component: TextField,
    args: {
        label: "Title",
        placeholder: "Untitled",
        size: "md",
        isDisabled: false,
        isInvalid: false,
    },
    argTypes: {
        size: { control: "inline-radio", options: SIZES },
        isDisabled: { control: "boolean" },
        isInvalid: { control: "boolean" },
    },
} satisfies Meta<typeof TextField>;

export default meta;

type Story = StoryObj<typeof meta>;

// Interactive: drive size/disabled/invalid from the Controls panel.
export const Playground: Story = {
    args: {
        defaultValue: "Sunrise over the delta",
        description: "Shown in the asset header.",
    },
};

// The state × size grid: rows are field states, columns are the four size rungs.
export const Matrix: Story = {
    render: () => (
        <div
            style={{
                display: "grid",
                gridTemplateColumns: `auto repeat(${SIZES.length}, minmax(0, 1fr))`,
                gap: "var(--alx-space-6)",
                alignItems: "start",
            }}
        >
            <span />
            {SIZES.map((size) => (
                <span key={size} className="alx-type-caption" style={{ textAlign: "center" }}>
                    {size}
                </span>
            ))}

            <span className="alx-type-caption">rest</span>
            {SIZES.map((size) => (
                <TextField key={size} label="Title" size={size} placeholder="Untitled" />
            ))}

            <span className="alx-type-caption">filled</span>
            {SIZES.map((size) => (
                <TextField key={size} label="Title" size={size} defaultValue="Sunrise" />
            ))}

            <span className="alx-type-caption">disabled</span>
            {SIZES.map((size) => (
                <TextField key={size} label="Title" size={size} defaultValue="Sunrise" isDisabled />
            ))}

            <span className="alx-type-caption">invalid</span>
            {SIZES.map((size) => (
                <TextField
                    key={size}
                    label="Title"
                    size={size}
                    defaultValue="Sunrise"
                    isInvalid
                    errorMessage="Title is required."
                />
            ))}
        </div>
    ),
};
