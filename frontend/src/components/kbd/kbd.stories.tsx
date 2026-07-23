import type { Meta, StoryObj } from "@storybook/react-vite";
import type { IconConcept } from "@/components/icon/icon";
import { Kbd, type KbdSize, type KbdStyle, KbdGroup } from "./kbd";

const STYLES: KbdStyle[] = ["flat", "keycap"];
const SIZES: { key: KbdSize; label: string }[] = [
    { key: "xs", label: "xs · 16 (dense)" },
    { key: "sm", label: "sm · 20 (menu)" },
    { key: "md", label: "md · 24" },
];

// The six modifier concepts render as vector icons; letters/words stay text.
const MODIFIERS: IconConcept[] = ["command", "shift", "option", "control", "return", "delete"];
const LETTERS = ["P", "K", "Esc"];

const meta = {
    title: "Primitives/Kbd",
    component: Kbd,
    args: { children: "K", style: "flat", size: "sm" },
    argTypes: {
        style: { control: "inline-radio", options: STYLES },
        size: { control: "inline-radio", options: SIZES.map((s) => s.key) },
        children: { control: "text" },
    },
} satisfies Meta<typeof Kbd>;

export default meta;

type Story = StoryObj<typeof meta>;

// Interactive: drive style + glyph from the Controls panel.
export const Playground: Story = {};

const col = { display: "flex", flexDirection: "column", gap: "var(--alx-space-3)" } as const;
const rowCenter = { display: "flex", alignItems: "center", gap: "var(--alx-space-2)" } as const;

// Both styles across single caps (modifier icons + text keys) and composed combos. A combo is a
// KbdGroup of one <kbd> per key (the shadcn model) — modifiers ride `icon`, letters ride children.
export const Matrix: Story = {
    render: () => (
        <div style={{ ...col, gap: "var(--alx-space-8)" }}>
            {STYLES.map((style) => (
                <div key={style} style={col}>
                    <span className="alx-type-caption">{style}</span>
                    <div style={rowCenter}>
                        {MODIFIERS.map((concept) => (
                            <Kbd key={concept} style={style} icon={concept} />
                        ))}
                        {LETTERS.map((key) => (
                            <Kbd key={key} style={style}>
                                {key}
                            </Kbd>
                        ))}
                    </div>
                    <div style={rowCenter}>
                        <KbdGroup>
                            <Kbd style={style} icon="command" />
                            <Kbd style={style}>K</Kbd>
                        </KbdGroup>
                        <KbdGroup>
                            <Kbd style={style} icon="command" />
                            <Kbd style={style} icon="shift" />
                            <Kbd style={style}>P</Kbd>
                        </KbdGroup>
                        <KbdGroup>
                            <Kbd style={style} icon="command" />
                            <Kbd style={style} icon="delete" />
                        </KbdGroup>
                    </div>
                </div>
            ))}
        </div>
    ),
};

// The size ladder — caps ride the D33 control-size bundle (xs 16 / sm 20 / md 24), each a tier of
// {mono text + height + icon} derived together. text-box-trim frees xs from the 16px mono line and
// centers the glyph. sm is the menu default; xs is for dense surfaces. Stops at md (mono ceiling 12px).
export const Sizes: Story = {
    render: () => (
        <div style={{ ...col, gap: "var(--alx-space-8)" }}>
            {STYLES.map((style) => (
                <div key={style} style={col}>
                    <span className="alx-type-caption">{style}</span>
                    {SIZES.map(({ key, label }) => (
                        <div key={key} style={rowCenter}>
                            <span className="alx-type-caption" style={{ minWidth: "10ch" }}>
                                {label}
                            </span>
                            <Kbd style={style} size={key} icon="command" />
                            <Kbd style={style} size={key}>
                                K
                            </Kbd>
                            <KbdGroup>
                                <Kbd style={style} size={key} icon="command" />
                                <Kbd style={style} size={key} icon="shift" />
                                <Kbd style={style} size={key}>
                                    P
                                </Kbd>
                            </KbdGroup>
                        </div>
                    ))}
                </div>
            ))}
        </div>
    ),
};
