import type { ColorLabel } from "@/_generated-types/enums";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { LabelSwatch } from "./label-swatch";

// The generated ColorLabel union. `orange` has no §5 role (dropped 2026-07-18) but
// still renders for XMP round-trip — kept here so the specimen shows the fallback.
const LABELS: ColorLabel[] = ["red", "yellow", "green", "blue", "purple", "orange"];

const meta = {
    title: "Primitives/LabelSwatch",
    component: LabelSwatch,
    args: { label: "red", "aria-label": "Red label" },
    argTypes: {
        label: { control: "inline-radio", options: LABELS },
        "aria-label": { control: "text" },
    },
} satisfies Meta<typeof LabelSwatch>;

export default meta;

type Story = StoryObj<typeof meta>;

// Interactive: pick a label color from the Controls panel.
export const Playground: Story = {};

// Every assignable color label — never color alone (§10), so each is named.
export const Swatches: Story = {
    render: () => (
        <div style={{ display: "flex", gap: "var(--alx-space-6)", alignItems: "flex-start" }}>
            {LABELS.map((label) => (
                <div
                    key={label}
                    style={{
                        display: "flex",
                        flexDirection: "column",
                        alignItems: "center",
                        gap: "var(--alx-space-2)",
                    }}
                >
                    <LabelSwatch label={label} aria-label={`${label} label`} />
                    <span className="alx-type-caption">{label}</span>
                </div>
            ))}
        </div>
    ),
};
