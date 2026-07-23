import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge, type BadgeHue, type BadgeSize, type BadgeStyle } from "./badge";

const STYLES: BadgeStyle[] = ["tint", "outline", "fill", "dot"];
const SIZES: BadgeSize[] = ["inline", "standard", "prominent"];
const HUES: BadgeHue[] = [
    "red",
    "peach",
    "orange",
    "amber",
    "lime",
    "green",
    "teal",
    "cyan",
    "blue",
    "indigo",
    "purple",
    "magenta",
    "gray",
];

// Every hue in the scale, each badge self-labeled with its hue name.
const MATRIX_HUES: { hue: BadgeHue; label: string }[] = HUES.map((hue) => ({ hue, label: hue }));

const meta = {
    title: "Primitives/Badge",
    component: Badge,
    args: { children: "Draft", style: "tint", size: "standard", hue: "blue" },
    argTypes: {
        style: { control: "inline-radio", options: STYLES },
        size: { control: "inline-radio", options: SIZES },
        hue: { control: "select", options: HUES },
        children: { control: "text" },
    },
} satisfies Meta<typeof Badge>;

export default meta;

type Story = StoryObj<typeof meta>;

// Interactive: drive style/size/hue/label from the Controls panel.
export const Playground: Story = {};

// The full style × hue grid — one grammar across every hue — plus the size ladder.
export const Matrix: Story = {
    render: () => (
        <div style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-8)" }}>
            <div style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-3)" }}>
                <span className="alx-type-caption">Style (rows) × hue (columns)</span>
                <div
                    style={{
                        display: "grid",
                        gridTemplateColumns: `auto repeat(${MATRIX_HUES.length}, max-content)`,
                        gap: "var(--alx-space-3)",
                        alignItems: "center",
                    }}
                >
                    {STYLES.map((style) => (
                        <div key={style} style={{ display: "contents" }}>
                            <span className="alx-type-caption">{style}</span>
                            {MATRIX_HUES.map(({ hue, label }) => (
                                <span key={hue} style={{ justifySelf: "center" }}>
                                    <Badge style={style} hue={hue}>
                                        {label}
                                    </Badge>
                                </span>
                            ))}
                        </div>
                    ))}
                </div>
            </div>

            <div style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-3)" }}>
                <span className="alx-type-caption">Size ladder (tint · blue)</span>
                <div style={{ display: "flex", gap: "var(--alx-space-4)", alignItems: "center" }}>
                    {SIZES.map((size) => (
                        <div
                            key={size}
                            style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-1)", alignItems: "center" }}
                        >
                            <Badge style="tint" hue="blue" size={size}>
                                Draft
                            </Badge>
                            <span className="alx-type-caption">{size}</span>
                        </div>
                    ))}
                </div>
            </div>
        </div>
    ),
};
