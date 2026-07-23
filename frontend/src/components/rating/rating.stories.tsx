import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import "@/i18n"; // resolves the aria-label keys the component reads via useTranslation
import { Rating, type RatingSize } from "./rating";

const SIZES: RatingSize[] = ["xs", "sm", "md", "lg"];

// 0 (unrated) · partial · full — the three states the readout must render legibly.
const MATRIX_VALUES: (number | null)[] = [0, 3, 5];

const meta = {
    title: "Primitives/Rating",
    component: Rating,
    args: { value: 3, size: "md" },
    argTypes: {
        value: { control: { type: "number", min: 0, max: 5, step: 1 } },
        size: { control: "inline-radio", options: SIZES },
    },
} satisfies Meta<typeof Rating>;

export default meta;

type Story = StoryObj<typeof meta>;

// A rating is controlled: local state holds the value, onChange proposes the next.
function InteractiveRating({ initial, size }: { initial: number | null; size?: RatingSize }) {
    const [value, setValue] = useState<number | null>(initial);
    return <Rating value={value} onChange={setValue} size={size} />;
}

// Interactive: click a star to rate, click the current value to clear.
export const Playground: Story = {
    render: (args) => <InteractiveRating initial={args.value} size={args.size} />,
};

// Values (rows) × size ladder (columns) — every cell live.
export const Matrix: Story = {
    render: () => (
        <div
            style={{
                display: "grid",
                gridTemplateColumns: `auto repeat(${SIZES.length}, max-content)`,
                gap: "var(--alx-space-4)",
                alignItems: "center",
            }}
        >
            <span />
            {SIZES.map((size) => (
                <span key={size} className="alx-type-caption" style={{ textAlign: "center" }}>
                    {size}
                </span>
            ))}
            {MATRIX_VALUES.map((value) => (
                <div key={String(value)} style={{ display: "contents" }}>
                    <span className="alx-type-caption">{value === 0 ? "unrated" : value}</span>
                    {SIZES.map((size) => (
                        <span key={size} style={{ justifySelf: "center" }}>
                            <InteractiveRating initial={value} size={size} />
                        </span>
                    ))}
                </div>
            ))}
        </div>
    ),
};
