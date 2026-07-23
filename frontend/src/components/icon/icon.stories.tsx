import type { Meta, StoryObj } from "@storybook/react-vite";
import { Icon, type IconConcept } from "./icon";

const CONCEPTS: IconConcept[] = ["check", "disclose", "mixed", "rating", "flag", "reject", "settings"];

const meta = {
    title: "Primitives/Icon",
    component: Icon,
    args: { concept: "check" },
    argTypes: {
        concept: { control: "inline-radio", options: CONCEPTS },
    },
} satisfies Meta<typeof Icon>;

export default meta;

type Story = StoryObj<typeof meta>;

// Interactive: pick a concept from the Controls panel. Icons ride currentColor.
export const Playground: Story = {};

// Every registered concept with its name — one glyph per meaning (§14).
export const Gallery: Story = {
    render: () => (
        <div
            style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(5rem, 1fr))",
                gap: "var(--alx-space-4)",
            }}
        >
            {CONCEPTS.map((concept) => (
                <div
                    key={concept}
                    style={{
                        display: "flex",
                        flexDirection: "column",
                        alignItems: "center",
                        gap: "var(--alx-space-2)",
                    }}
                >
                    <Icon concept={concept} />
                    <span className="alx-type-caption">{concept}</span>
                </div>
            ))}
        </div>
    ),
};
